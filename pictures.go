package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
)

const (
	catalogURL       = "https://api.lolicon.app/setu/v2?size=original&r18=0&excludeAI=true&proxy=i.pximg.net"
	imageHost        = "i.pximg.net"
	downloadCapacity = 3
	maxCatalogRounds = 3
)

type picture struct {
	content []byte
	mime    string
}

type candidate struct {
	R18  bool `json:"r18"`
	URLs struct {
		Original string `json:"original"`
	} `json:"urls"`
}

type pictureSource struct {
	client *http.Client
	// slots 由同一插件实例的所有请求共享，包括串行取图的请求。
	slots chan struct{}
}

func newPictureSource() *pictureSource {
	return &pictureSource{
		client: &http.Client{
			// 目录和原图都使用固定源，重定向作为上游返回状态处理。
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		slots: make(chan struct{}, downloadCapacity),
	}
}

func (source *pictureSource) collect(ctx context.Context, count int) ([]picture, error) {
	pictures := make([]picture, 0, count)
	var failures []error
	for round := 0; round < maxCatalogRounds; round++ {
		missing := count - len(pictures)
		candidates, err := source.random(ctx, missing)
		if err != nil {
			return nil, errors.Join(append(failures, err)...)
		}
		// 目录按 num 返回候选；多余条目无需下载。
		candidates = candidates[:min(missing, len(candidates))]
		batch, err := source.downloadBatch(ctx, candidates, count > 2)
		if err != nil {
			return nil, errors.Join(append(failures, err)...)
		}
		for _, result := range batch {
			if result.err != nil {
				failures = append(failures, result.err)
				continue
			}
			pictures = append(pictures, result.picture)
		}
		if len(pictures) == count {
			return pictures, nil
		}
	}
	return nil, fmt.Errorf("collected %d of %d pictures after %d rounds: %w",
		len(pictures), count, maxCatalogRounds, errors.Join(append(failures, errors.New("not enough usable pictures"))...))
}

func (source *pictureSource) random(ctx context.Context, count int) ([]candidate, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, catalogURL+"&num="+strconv.Itoa(count), nil)
	if err != nil {
		return nil, err
	}
	response, err := source.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("request picture catalog: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, responseError("request picture catalog", response)
	}
	var result struct {
		Error string      `json:"error"`
		Data  []candidate `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode picture catalog: %w", err)
	}
	if result.Error != "" {
		return nil, fmt.Errorf("picture catalog: %s", result.Error)
	}
	return result.Data, nil
}

type downloadResult struct {
	picture picture
	err     error
}

func (source *pictureSource) downloadBatch(ctx context.Context, candidates []candidate, parallel bool) ([]downloadResult, error) {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	workers := 1
	if parallel {
		workers = min(downloadCapacity, len(candidates))
	}
	results := make([]downloadResult, len(candidates))
	queue := make(chan int, len(candidates))
	for index := range candidates {
		queue <- index
	}
	close(queue)
	work := func() {
		for index := range queue {
			if ctx.Err() != nil {
				return
			}
			picture, err := source.download(ctx, candidates[index])
			var status *httpStatusError
			if errors.As(err, &status) && (status.code >= 500 || status.code == http.StatusTooManyRequests) {
				cancel(err)
				return
			}
			results[index] = downloadResult{picture, err}
		}
	}
	if workers <= 1 {
		work()
	} else {
		var group sync.WaitGroup
		for range workers {
			group.Go(work)
		}
		group.Wait()
	}
	// 用候选次序组装结果，下载完成顺序不会改变节点顺序。
	return results, context.Cause(ctx)
}

func (source *pictureSource) download(ctx context.Context, entry candidate) (picture, error) {
	if entry.R18 {
		return picture{}, errors.New("catalog returned an adult picture with r18=0")
	}
	link, err := url.Parse(entry.URLs.Original)
	if err != nil {
		return picture{}, fmt.Errorf("parse original picture URL: %w", err)
	}
	if link.Scheme != "https" || link.Host != imageHost || link.User != nil {
		return picture{}, errors.New("original picture URL must use the configured HTTPS image host")
	}
	select {
	case source.slots <- struct{}{}:
		defer func() { <-source.slots }()
	case <-ctx.Done():
		return picture{}, context.Cause(ctx)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, link.String(), nil)
	if err != nil {
		return picture{}, err
	}
	// Pixiv 原图要求来源头；直接用经验证的字节发送，适配器无需访问此链接。
	request.Header.Set("Referer", "https://www.pixiv.net/")
	response, err := source.client.Do(request)
	if err != nil {
		return picture{}, fmt.Errorf("download original picture: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return picture{}, responseError("download original picture", response)
	}
	content, err := io.ReadAll(response.Body)
	if err != nil {
		return picture{}, fmt.Errorf("read original picture: %w", err)
	}
	mime := http.DetectContentType(content)
	if !strings.HasPrefix(mime, "image/") {
		return picture{}, fmt.Errorf("original picture response contains %s", mime)
	}
	return picture{content: content, mime: mime}, nil
}

type httpStatusError struct {
	code int
	err  error
}

func (err *httpStatusError) Error() string { return err.err.Error() }
func (err *httpStatusError) Unwrap() error { return err.err }

func responseError(operation string, response *http.Response) error {
	status := &httpStatusError{code: response.StatusCode}
	body, err := io.ReadAll(response.Body)
	status.err = fmt.Errorf("%s: HTTP %d: %s", operation, status.code, body)
	if err != nil {
		status.err = errors.Join(status.err, fmt.Errorf("read error response: %w", err))
	}
	return status
}
