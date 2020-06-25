package proxy

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/adamluzsi/testcase"
)

func newServer() {

}

type spyHandler struct {
	calls    int
	respCode int
	mu       sync.Mutex
}

func (h *spyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls++
	w.WriteHeader(h.respCode)
}

type resErr struct {
	res *http.Response
	err error
}

func TestProxy(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Describe("Proxy", SpecProxy)
	//s.Describe("ListenAndTransfer", SpecProxyListenAndTransfer)
}

func SpecProxy(s *testcase.Spec) {
	subject := func(t *testcase.T) error {
		upstreamNetwork := t.I(`upstreamNetwork`).(string)
		upstreamAddr := t.I(`upstreamAddr`).(string)
		upstreamTLS := t.I(`upstreamTLS`).(bool)
		downstreamLis := t.I(`downstreamLis`).(net.Listener)

		conn, err := downstreamLis.Accept()
		if err != nil {
			t.Fatalf("cannot accept conn from downstream: %v", err)
		}

		proxy := Proxy{
			UpstreamNetwork: upstreamNetwork,
			UpstreamAddr:    upstreamAddr,
			UpstreamTLS:     upstreamTLS,
		}

		return proxy.Proxy(conn)
	}

	s.Let(`downstreamLis`, func(t *testcase.T) interface{} {
		lis, err := net.Listen("tcp", "")
		if err != nil {
			t.Fatalf("cannot listen tcp: %v", err)
		}
		t.Defer(lis.Close)
		return lis
	})

	s.When(`upstream is an http server`, func(s *testcase.Spec) {
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
					lis := t.I(`downstreamLis`).(net.Listener)

					var wg sync.WaitGroup

					var prxErr error
					wg.Add(1)
					go func() {
						defer wg.Done()
						prxErr = subject(t)
					}()

					var _ *http.Response
					var _ error
					wg.Add(1)
					go func() {
						defer wg.Done()
						_, _ = http.Get(fmt.Sprintf("http://%s/", lis.Addr().String()))
					}()

					wg.Wait()

					if prxErr != nil {
						t.Errorf("got proxy err = %v, want proxy err = <nil>", prxErr)
					}
				})
			})

			s.And(`downstream sends https request`, func(s *testcase.Spec) {
				http.Get("https://localhost:31234")
			})

			s.And(`downstream sends non http request`, func(s *testcase.Spec) {
				//conn, _ := net.Dial("tcp", "localhost:31234")
				//_, _ = conn.Write([]byte(`some randaom gibberish`))
			})
		})

		s.And(`it supports tls`, func(s *testcase.Spec) {
			s.Around(func(t *testcase.T) func() {
				upstream := t.I(`upstream`).(*httptest.Server)
				upstream.StartTLS()
				return upstream.Close
			})

			s.And(`downstream sends http request without upgrade header`, func(s *testcase.Spec) {

			})

			s.And(`downstream sends http request with upgrade header`, func(s *testcase.Spec) {

			})

			s.And(`downstream sends https request`, func(s *testcase.Spec) {

			})

			s.And(`downstream sends non http request with first byte as 22`, func(s *testcase.Spec) {

			})

			s.And(`downstream sends any non http request`, func(s *testcase.Spec) {

			})
		})
	})
}
