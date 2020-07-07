package proxy

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/adamluzsi/testcase"
)

type spyHandler struct {
	calls    int
	respCode int
	mu       sync.Mutex
}

func (h *spyHandler) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls++
	w.WriteHeader(h.respCode)
}

func TestProxy(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Describe("Transfer", SpecTransfer)
}

func SpecTransfer(s *testcase.Spec) {
	s.Let(`proxy`, func(t *testcase.T) interface{} {
		upstreamNetwork := t.I(`upstreamNetwork`).(string)
		upstreamAddr := t.I(`upstreamAddr`).(string)
		upstreamTLS := t.I(`upstreamTLS`).(bool)

		proxy := Proxy{
			UpstreamNetwork: upstreamNetwork,
			UpstreamAddr:    upstreamAddr,
			UpstreamTLS:     upstreamTLS,
		}

		return &proxy
	})

	s.Let(`downstreamLis`, func(t *testcase.T) interface{} {
		lis, err := net.Listen("tcp", "localhost:31234")
		if err != nil {
			t.Fatalf("cannot listen downstream tcp connections: %v", err)
		}

		t.Defer(lis.Close)
		return lis
	})

	s.Around(func(t *testcase.T) func() {
		proxy := t.I(`proxy`).(*Proxy)
		lis := t.I(`downstreamLis`).(net.Listener)
		wg := t.I(`wg`).(*sync.WaitGroup)

		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := proxy.Transfer(lis); err != nil && !errors.Is(err, ErrClosed) {
				t.Errorf("unexpected error while transferring: %v", err)
			}
		}()
		runtime.Gosched()

		return func() {
			if err := proxy.Shutdown(context.Background()); err != nil && !errors.Is(err, ErrNotRunning) {
				t.Errorf("unexpected error while shutting down proxy: %v", err)
			}
		}
	})

	s.When(`proxying connections from downstream listener`, func(s *testcase.Spec) {
		s.Let(`wg`, func(t *testcase.T) interface{} {
			return &sync.WaitGroup{}
		})

		s.And(`upstream is an http server`, func(s *testcase.Spec) {
			s.Let(`handler`, func(t *testcase.T) interface{} {
				return &spyHandler{respCode: http.StatusTeapot}
			})

			s.Let(`upstream`, func(t *testcase.T) interface{} {
				handler := t.I(`handler`).(http.Handler)
				srv := httptest.NewUnstartedServer(handler)
				return srv
			})
			s.Let(`upstreamNetwork`, func(t *testcase.T) interface{} {
				upstream := t.I(`upstream`).(*httptest.Server)
				return upstream.Listener.Addr().Network()
			})
			s.Let(`upstreamAddr`, func(t *testcase.T) interface{} {
				upstream := t.I(`upstream`).(*httptest.Server)
				return upstream.Listener.Addr().String()
			})

			s.And(`it does not support tls`, func(s *testcase.Spec) {
				s.LetValue(`upstreamTLS`, false)

				s.Around(func(t *testcase.T) func() {
					upstream := t.I(`upstream`).(*httptest.Server)
					upstream.Start()
					return upstream.Close
				})

				s.And(`downstream sends http request`, func(s *testcase.Spec) {
					s.Then(`it should return http response with status code from upstream`, func(t *testcase.T) {
						addr := t.I(`downstreamLis`).(net.Listener).Addr()
						wg := t.I(`wg`).(*sync.WaitGroup)

						handler := t.I(`handler`).(*spyHandler)

						var err error
						var res *http.Response
						wg.Add(1)
						go func() {
							defer wg.Done()
							res, err = http.Get(fmt.Sprintf("http://%s/", addr))
						}()

						time.Sleep(100 * time.Millisecond)
						proxy := t.I(`proxy`).(*Proxy)
						ctx := context.Background()
						if err := proxy.Shutdown(ctx); err != nil {
							t.Errorf("unexpected error while shutting down proxy: %v", err)
						}

						wg.Wait()

						if err != nil {
							t.Fatalf("got http get err = %v, want http get err = <nil>", err)
						}

						if res.StatusCode != handler.respCode {
							t.Errorf("got http status code = %v, want http status code = %v", res.StatusCode, handler.respCode)
						}

						if handler.calls != 1 {
							t.Errorf("got http handler calls = %v, want http handler calls = %v", handler.calls, 1)
						}
					})
				})

				s.And(`downstream sends https request`, func(s *testcase.Spec) {
					s.Then(`it should return a non nil error`, func(t *testcase.T) {
						addr := t.I(`downstreamLis`).(net.Listener).Addr()
						wg := t.I(`wg`).(*sync.WaitGroup)

						handler := t.I(`handler`).(*spyHandler)

						var err error
						wg.Add(1)
						go func() {
							defer wg.Done()
							_, err = http.Get(fmt.Sprintf("https://%s/", addr))
						}()

						time.Sleep(100 * time.Millisecond)
						proxy := t.I(`proxy`).(*Proxy)
						ctx := context.Background()
						if err := proxy.Shutdown(ctx); err != nil {
							t.Errorf("unexpected error while shutting down proxy: %v", err)
						}

						wg.Wait()

						if err == nil {
							t.Errorf("got http get err = %v, want http get err = <non-nil>", err)
						}

						if handler.calls != 0 {
							t.Errorf("got http handler calls = %v, want http handler calls = %v", handler.calls, 0)
						}
					})
				})
			})

			s.And(`it supports tls`, func(s *testcase.Spec) {
				s.LetValue(`upstreamTLS`, true)
				s.Around(func(t *testcase.T) func() {
					upstream := t.I(`upstream`).(*httptest.Server)
					upstream.StartTLS()
					return upstream.Close
				})

				s.Let(`httpClient`, func(t *testcase.T) interface{} {
					return &http.Client{
						Transport: &http.Transport{
							TLSClientConfig: &tls.Config{
								InsecureSkipVerify: true,
							},
						},
					}
				})

				s.And(`downstream sends http request without upgrade header`, func(s *testcase.Spec) {
					s.Then(`it should return non nil error`, func(t *testcase.T) {
						addr := t.I(`downstreamLis`).(net.Listener).Addr()
						wg := t.I(`wg`).(*sync.WaitGroup)

						handler := t.I(`handler`).(*spyHandler)

						httpClient := t.I(`httpClient`).(*http.Client)

						var err error
						var _ *http.Response
						wg.Add(1)
						go func() {
							defer wg.Done()
							_, err = httpClient.Get(fmt.Sprintf("http://%s/", addr))
						}()

						time.Sleep(100 * time.Millisecond)
						proxy := t.I(`proxy`).(*Proxy)
						ctx := context.Background()
						if err := proxy.Shutdown(ctx); err != nil {
							t.Errorf("unexpected error while shutting down proxy: %v", err)
						}

						wg.Wait()

						if err == nil {
							t.Errorf("got http get err = %v, want http get err = <non-nil>", err)
						}

						if handler.calls != 0 {
							t.Errorf("got http handler calls = %v, want http handler calls = %v", handler.calls, 0)
						}
					})
				})

				s.And(`downstream sends http request with upgrade header`, func(s *testcase.Spec) {
					s.Then(`it should return http response with status code from upstream`, func(t *testcase.T) {
						addr := t.I(`downstreamLis`).(net.Listener).Addr()
						wg := t.I(`wg`).(*sync.WaitGroup)

						handler := t.I(`handler`).(*spyHandler)

						httpClient := t.I(`httpClient`).(*http.Client)
						req, reqErr := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s/", addr), nil)
						if reqErr != nil {
							t.Errorf("unexpected error while creating request: %v", reqErr)
						}
						req.Header.Add(headerUpgradeInsecureRequests, "1")

						var err error
						var res *http.Response
						wg.Add(1)
						go func() {
							defer wg.Done()
							res, err = httpClient.Do(req)
						}()

						time.Sleep(100 * time.Millisecond)
						proxy := t.I(`proxy`).(*Proxy)
						ctx := context.Background()
						if err := proxy.Shutdown(ctx); err != nil {
							t.Errorf("unexpected error while shutting down proxy: %v", err)
						}

						wg.Wait()

						if err != nil {
							t.Fatalf("got http get err = %v, want http get err = <nil>", err)
						}

						if res.StatusCode != handler.respCode {
							t.Errorf("got http status code = %v, want http status code = %v", res.StatusCode, handler.respCode)
						}

						if handler.calls != 1 {
							t.Errorf("got http handler calls = %v, want http handler calls = %v", handler.calls, 1)
						}
					})
				})

				s.And(`downstream sends https request`, func(s *testcase.Spec) {
					s.Then(`it should return http response with status code from upstream`, func(t *testcase.T) {
						addr := t.I(`downstreamLis`).(net.Listener).Addr()
						wg := t.I(`wg`).(*sync.WaitGroup)

						handler := t.I(`handler`).(*spyHandler)

						httpClient := t.I(`httpClient`).(*http.Client)

						var err error
						var res *http.Response
						wg.Add(1)
						go func() {
							defer wg.Done()
							res, err = httpClient.Get(fmt.Sprintf("https://%s/", addr))
						}()

						time.Sleep(100 * time.Millisecond)
						proxy := t.I(`proxy`).(*Proxy)
						ctx := context.Background()
						if err := proxy.Shutdown(ctx); err != nil {
							t.Errorf("unexpected error while shutting down proxy: %v", err)
						}

						wg.Wait()

						if err != nil {
							t.Fatalf("got http get err = %v, want http get err = <nil>", err)
						}

						if res.StatusCode != handler.respCode {
							t.Errorf("got http status code = %v, want http status code = %v", res.StatusCode, handler.respCode)
						}

						if handler.calls != 1 {
							t.Errorf("got http handler calls = %v, want http handler calls = %v", handler.calls, 1)
						}
					})
				})
			})
		})
	})
}
