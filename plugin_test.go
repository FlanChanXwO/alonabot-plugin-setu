package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlonaBot/AlonaBot/plugintest"
	"github.com/AlonaBot/AlonaBot/sdk/errcode"
	"github.com/AlonaBot/AlonaBot/sdk/message"
)

type fixtureTransport struct {
	origin *url.URL
}

func (transport fixtureTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	local := request.Clone(request.Context())
	local.URL.Scheme = transport.origin.Scheme
	local.URL.Host = transport.origin.Host
	return http.DefaultTransport.RoundTrip(local)
}

func fixtureSource(t *testing.T, handler http.HandlerFunc) *pictureSource {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	origin, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	source := newPictureSource()
	source.client.Transport = fixtureTransport{origin}
	return source
}

func writeCatalog(w http.ResponseWriter, paths ...string) {
	entries := make([]candidate, 0, len(paths))
	for _, path := range paths {
		var entry candidate
		entry.URLs.Original = "https://i.pximg.net/" + path
		entries = append(entries, entry)
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"data": entries})
}

func requirePictures(t *testing.T, result plugintest.Result, count int, forwarded bool) {
	t.Helper()
	if len(result.Sends) != 1 {
		t.Fatalf("expected one send, got %+v", result.Sends)
	}
	chain := result.Sends[0].Chain
	if forwarded {
		if len(chain) != 1 {
			t.Fatalf("expected one forward segment, got %v", chain)
		}
		forward, ok := chain[0].(message.ForwardSegment)
		if !ok || len(forward.Items) != count {
			t.Fatalf("expected %d forward nodes, got %v", count, chain)
		}
		chain = nil
		for _, node := range forward.Items {
			if len(node.Chain) != 1 {
				t.Fatalf("expected one image per node, got %v", node.Chain)
			}
			chain = append(chain, node.Chain...)
		}
	}
	if len(chain) != count {
		t.Fatalf("expected %d pictures, got %v", count, chain)
	}
	for _, segment := range chain {
		if image, ok := segment.(message.ImageSegment); !ok || image.Source.MediaRef == "" {
			t.Fatalf("expected managed picture, got %v", segment)
		}
	}
}

func TestRoutesThroughCore(t *testing.T) {
	for _, test := range []struct {
		mode, platform string
		inputs         []string
		counts         []int
		forwarded      []bool
	}{
		{"", "onebot11", []string{"来份色图", "来1份色图", "来二份色图", "来三份色图"}, []int{1, 1, 2, 3}, []bool{false, false, false, false}},
		{"multi", "onebot11", []string{"来份色图", "来两份色图"}, []int{1, 2}, []bool{false, true}},
		{"always", "onebot11", []string{"来份色图", "来三份色图"}, []int{1, 3}, []bool{true, true}},
		{"always", "telegram", []string{"来三份色图"}, []int{3}, []bool{false}},
	} {
		t.Run(test.platform+"/"+test.mode, func(t *testing.T) {
			source := fixtureSource(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/setu/v2" {
					n, _ := strconv.Atoi(r.URL.Query().Get("num"))
					paths := make([]string, n)
					for index := range paths {
						paths[index] = "original-" + strconv.Itoa(index)
					}
					writeCatalog(w, paths...)
					return
				}
				if r.Referer() != "https://www.pixiv.net/" {
					http.Error(w, "missing referer", http.StatusForbidden)
					return
				}
				_ = png.Encode(w, image.NewRGBA(image.Rect(0, 0, 1, 1)))
			})
			plugin := newPlugin(source)
			options := []plugintest.Option{plugintest.Plugin(plugin), plugintest.Adapter(test.platform)}
			if test.mode != "" {
				options = append(options, plugintest.Config(plugin, fmt.Sprintf(`{"forward_mode":%q}`, test.mode)))
			}
			harness := plugintest.Start(t, options...)
			user := plugintest.User{ID: "alice"}
			chat := plugintest.Private(user.ID)
			for index, input := range test.inputs {
				requirePictures(t, harness.Text(chat, user, input), test.counts[index], test.forwarded[index])
			}
			if result := harness.Text(chat, user, "/setu"); len(result.Sends) != 0 || len(result.Outcomes) != 0 {
				t.Fatalf("legacy command still registered: %+v", result)
			}
			harness.Text(chat, user, "来11份色图").RequireReply(t, "一次可以请求 1 到 10 张色图。")
		})
	}
}

func TestFailuresThroughCore(t *testing.T) {
	for _, test := range []struct {
		scenario string
		rounds   int32
		success  bool
	}{
		{"replace", 2, true},
		{"exhaust", 3, false},
		{"catalog 503", 1, false},
		{"image 503", 1, false},
		{"send fails", 1, false},
	} {
		t.Run(test.scenario, func(t *testing.T) {
			var rounds atomic.Int32
			source := fixtureSource(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/setu/v2":
					round := rounds.Add(1)
					if test.scenario == "catalog 503" {
						http.Error(w, "catalog unavailable", http.StatusServiceUnavailable)
						return
					}
					if got := r.URL.Query().Get("num"); got != "1" && !(round == 1 && test.scenario == "replace" && got == "2") {
						t.Errorf("unexpected catalog num %q at round %d", got, round)
					}
					switch {
					case test.scenario == "replace" && round == 1:
						writeCatalog(w, "good", "missing")
					case test.scenario == "exhaust":
						writeCatalog(w, "html")
					case test.scenario == "image 503":
						writeCatalog(w, "unavailable")
					default:
						writeCatalog(w, "good")
					}
				case "/good":
					_ = png.Encode(w, image.NewRGBA(image.Rect(0, 0, 1, 1)))
				case "/missing":
					http.NotFound(w, r)
				case "/unavailable":
					http.Error(w, "image CDN unavailable", http.StatusServiceUnavailable)
				case "/html":
					_, _ = fmt.Fprint(w, "<html>not an image</html>")
				}
			})
			var failedSends atomic.Int32
			harness := plugintest.Start(t, plugintest.Plugin(newPlugin(source)), plugintest.Operations(plugintest.Behavior{
				Send: func(_ context.Context, call plugintest.Call) (*message.SendResult, *errcode.Error) {
					if test.scenario == "send fails" && call.Chain.Text() == "" {
						failedSends.Add(1)
						return nil, errcode.Unavailable.New("platform rejected pictures")
					}
					return call.SendResult, nil
				},
			}))
			user := plugintest.User{ID: "alice"}
			chat := plugintest.Private(user.ID)
			if test.success {
				requirePictures(t, harness.Text(chat, user, "来二份色图"), 2, false)
			} else {
				result := harness.Text(chat, user, "来份色图")
				if test.scenario == "send fails" {
					if len(result.Sends) != 2 || result.Sends[0].Err == nil || result.Sends[1].Err != nil {
						t.Fatalf("expected a rejected send and one successful notice: %+v", result.Sends)
					}
					result.Sends = result.Sends[1:]
				}
				result.RequireReply(t, "这次没能发出色图，请稍后再试。")
			}
			if got := rounds.Load(); got != test.rounds {
				t.Fatalf("catalog rounds = %d, want %d", got, test.rounds)
			}
			if test.scenario == "send fails" && failedSends.Load() != 1 {
				t.Fatalf("attempted image sends = %d, want 1", failedSends.Load())
			}
		})
	}
}

func TestDownloadQueue(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	started := make(chan struct{}, 10)
	release := make(chan struct{})
	defer func() { close(release) }()

	var active, peak atomic.Int32
	source := fixtureSource(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/setu/v2" {
			writeCatalog(w, "a", "b", "c", "d", "e")
			return
		}
		current := active.Add(1)
		defer active.Add(-1)
		for previous := peak.Load(); current > previous; previous = peak.Load() {
			if peak.CompareAndSwap(previous, current) {
				break
			}
		}
		started <- struct{}{}
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		_ = png.Encode(w, image.NewRGBA(image.Rect(0, 0, 1, 1)))
	})

	done := make(chan error, 2)
	for range 2 {
		go func() {
			pictures, err := source.collect(ctx, 5)
			if err == nil && len(pictures) != 5 {
				err = fmt.Errorf("received %d pictures, want 5", len(pictures))
			}
			done <- err
		}()
	}
	for range 3 {
		select {
		case <-started:
		case <-ctx.Done():
			t.Fatal("three downloads did not overlap")
		}
	}
	for range 10 {
		select {
		case release <- struct{}{}:
		case <-ctx.Done():
			t.Fatal("download queue did not drain")
		}
	}
	for range 2 {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	if got := peak.Load(); got != 3 {
		t.Fatalf("peak across two requests = %d, want 3", got)
	}
}

func TestConfiguredPictureLimitThroughCore(t *testing.T) {
	var requests atomic.Int32
	source := fixtureSource(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path == "/setu/v2" {
			if got := r.URL.Query().Get("num"); got != "2" {
				t.Errorf("catalog num = %q, want 2", got)
			}
			writeCatalog(w, "first", "second")
			return
		}
		_ = png.Encode(w, image.NewRGBA(image.Rect(0, 0, 1, 1)))
	})
	plugin := newPlugin(source)
	harness := plugintest.Start(t, plugintest.Plugin(plugin), plugintest.Config(plugin, `{"max_pictures":2}`))
	user := plugintest.User{ID: "alice"}
	chat := plugintest.Private(user.ID)
	harness.Text(chat, user, "来三份色图").RequireReply(t, "一次可以请求 1 到 2 张色图。")
	if got := requests.Load(); got != 0 {
		t.Fatalf("above-limit request contacted upstream %d times", got)
	}
	requirePictures(t, harness.Text(chat, user, "来二份色图"), 2, false)
}
