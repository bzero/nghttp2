package nghttp2

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"golang.org/x/net/http2/hpack"
	"golang.org/x/net/websocket"
	"io"
	"net/http"
	"syscall"
	"testing"
	"time"
)

// TestH1H1PlainGET tests whether simple HTTP/1 GET request works.
func TestH1H1PlainGET(t *testing.T) {
	st := newServerTester(nil, t, noopHandler)
	defer st.Close()

	res, err := st.http1(requestParam{
		name: "TestH1H1PlainGET",
	})
	if err != nil {
		t.Fatalf("Error st.http1() = %v", err)
	}

	want := 200
	if got := res.status; got != want {
		t.Errorf("status = %v; want %v", got, want)
	}
}

// TestH1H1PlainGETClose tests whether simple HTTP/1 GET request with
// Connetion: close request header field works.
func TestH1H1PlainGETClose(t *testing.T) {
	st := newServerTester(nil, t, noopHandler)
	defer st.Close()

	res, err := st.http1(requestParam{
		name: "TestH1H1PlainGETClose",
		header: []hpack.HeaderField{
			pair("Connection", "close"),
		},
	})
	if err != nil {
		t.Fatalf("Error st.http1() = %v", err)
	}

	want := 200
	if got := res.status; got != want {
		t.Errorf("status = %v; want %v", got, want)
	}
}

// TestH1H1InvalidMethod tests that server rejects invalid method with
// 501 status code
func TestH1H1InvalidMethod(t *testing.T) {
	st := newServerTester(nil, t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("server should not forward this request")
	})
	defer st.Close()

	res, err := st.http1(requestParam{
		name:   "TestH1H1InvalidMethod",
		method: "get",
	})
	if err != nil {
		t.Fatalf("Error st.http1() = %v", err)
	}

	if got, want := res.status, 501; got != want {
		t.Errorf("status = %v; want %v", got, want)
	}
}

// TestH1H1MultipleRequestCL tests that server rejects request which
// contains multiple Content-Length header fields.
func TestH1H1MultipleRequestCL(t *testing.T) {
	st := newServerTester(nil, t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("server should not forward bad request")
	})
	defer st.Close()

	if _, err := io.WriteString(st.conn, fmt.Sprintf(`GET / HTTP/1.1
Host: %v
Test-Case: TestH1H1MultipleRequestCL
Content-Length: 0
Content-Length: 0

`, st.authority)); err != nil {
		t.Fatalf("Error io.WriteString() = %v", err)
	}

	resp, err := http.ReadResponse(bufio.NewReader(st.conn), nil)
	if err != nil {
		t.Fatalf("Error http.ReadResponse() = %v", err)
	}

	want := 400
	if got := resp.StatusCode; got != want {
		t.Errorf("status: %v; want %v", got, want)
	}
}

// // TestH1H1ConnectFailure tests that server handles the situation that
// // connection attempt to HTTP/1 backend failed.
// func TestH1H1ConnectFailure(t *testing.T) {
// 	st := newServerTester(nil, t, noopHandler)
// 	defer st.Close()

// 	// shutdown backend server to simulate backend connect failure
// 	st.ts.Close()

// 	res, err := st.http1(requestParam{
// 		name: "TestH1H1ConnectFailure",
// 	})
// 	if err != nil {
// 		t.Fatalf("Error st.http1() = %v", err)
// 	}
// 	want := 503
// 	if got := res.status; got != want {
// 		t.Errorf("status: %v; want %v", got, want)
// 	}
// }

// TestH1H1GracefulShutdown tests graceful shutdown.
func TestH1H1GracefulShutdown(t *testing.T) {
	st := newServerTester(nil, t, noopHandler)
	defer st.Close()

	res, err := st.http1(requestParam{
		name: "TestH1H1GracefulShutdown-1",
	})
	if err != nil {
		t.Fatalf("Error st.http1() = %v", err)
	}

	if got, want := res.status, 200; got != want {
		t.Errorf("status: %v; want %v", got, want)
	}

	st.cmd.Process.Signal(syscall.SIGQUIT)
	time.Sleep(150 * time.Millisecond)

	res, err = st.http1(requestParam{
		name: "TestH1H1GracefulShutdown-2",
	})
	if err != nil {
		t.Fatalf("Error st.http1() = %v", err)
	}

	if got, want := res.status, 200; got != want {
		t.Errorf("status: %v; want %v", got, want)
	}

	if got, want := res.connClose, true; got != want {
		t.Errorf("res.connClose: %v; want %v", got, want)
	}

	want := io.EOF
	if _, err := st.conn.Read(nil); err == nil || err != want {
		t.Errorf("st.conn.Read(): %v; want %v", err, want)
	}
}

// TestH1H1HostRewrite tests that server rewrites Host header field
func TestH1H1HostRewrite(t *testing.T) {
	st := newServerTester([]string{"--host-rewrite"}, t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("request-host", r.Host)
	})
	defer st.Close()

	res, err := st.http1(requestParam{
		name: "TestH1H1HostRewrite",
	})
	if err != nil {
		t.Fatalf("Error st.http1() = %v", err)
	}
	if got, want := res.status, 200; got != want {
		t.Errorf("status: %v; want %v", got, want)
	}
	if got, want := res.header.Get("request-host"), st.backendHost; got != want {
		t.Errorf("request-host: %v; want %v", got, want)
	}
}

// TestH1H1BadHost tests that server rejects request including bad
// characters in host header field.
func TestH1H1BadHost(t *testing.T) {
	st := newServerTester(nil, t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("server should not forward this request")
	})
	defer st.Close()

	if _, err := io.WriteString(st.conn, "GET / HTTP/1.1\r\nTest-Case: TestH1H1HBadHost\r\nHost: foo\"bar\r\n\r\n"); err != nil {
		t.Fatalf("Error io.WriteString() = %v", err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(st.conn), nil)
	if err != nil {
		t.Fatalf("Error http.ReadResponse() = %v", err)
	}
	if got, want := resp.StatusCode, 400; got != want {
		t.Errorf("status: %v; want %v", got, want)
	}
}

// TestH1H1BadAuthority tests that server rejects request including
// bad characters in authority component of requset URI.
func TestH1H1BadAuthority(t *testing.T) {
	st := newServerTester(nil, t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("server should not forward this request")
	})
	defer st.Close()

	if _, err := io.WriteString(st.conn, "GET http://foo\"bar/ HTTP/1.1\r\nTest-Case: TestH1H1HBadAuthority\r\nHost: foobar\r\n\r\n"); err != nil {
		t.Fatalf("Error io.WriteString() = %v", err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(st.conn), nil)
	if err != nil {
		t.Fatalf("Error http.ReadResponse() = %v", err)
	}
	if got, want := resp.StatusCode, 400; got != want {
		t.Errorf("status: %v; want %v", got, want)
	}
}

// TestH1H1BadScheme tests that server rejects request including
// bad characters in scheme component of requset URI.
func TestH1H1BadScheme(t *testing.T) {
	st := newServerTester(nil, t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("server should not forward this request")
	})
	defer st.Close()

	if _, err := io.WriteString(st.conn, "GET http*://example.com/ HTTP/1.1\r\nTest-Case: TestH1H1HBadScheme\r\nHost: example.com\r\n\r\n"); err != nil {
		t.Fatalf("Error io.WriteString() = %v", err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(st.conn), nil)
	if err != nil {
		t.Fatalf("Error http.ReadResponse() = %v", err)
	}
	if got, want := resp.StatusCode, 400; got != want {
		t.Errorf("status: %v; want %v", got, want)
	}
}

// TestH1H1HTTP10 tests that server can accept HTTP/1.0 request
// without Host header field
func TestH1H1HTTP10(t *testing.T) {
	st := newServerTester(nil, t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("request-host", r.Host)
	})
	defer st.Close()

	if _, err := io.WriteString(st.conn, "GET / HTTP/1.0\r\nTest-Case: TestH1H1HTTP10\r\n\r\n"); err != nil {
		t.Fatalf("Error io.WriteString() = %v", err)
	}

	resp, err := http.ReadResponse(bufio.NewReader(st.conn), nil)
	if err != nil {
		t.Fatalf("Error http.ReadResponse() = %v", err)
	}

	if got, want := resp.StatusCode, 200; got != want {
		t.Errorf("status: %v; want %v", got, want)
	}
	if got, want := resp.Header.Get("request-host"), st.backendHost; got != want {
		t.Errorf("request-host: %v; want %v", got, want)
	}
}

// TestH1H1HTTP10NoHostRewrite tests that server generates host header
// field using actual backend server even if --no-http-rewrite is
// used.
func TestH1H1HTTP10NoHostRewrite(t *testing.T) {
	st := newServerTester(nil, t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("request-host", r.Host)
	})
	defer st.Close()

	if _, err := io.WriteString(st.conn, "GET / HTTP/1.0\r\nTest-Case: TestH1H1HTTP10NoHostRewrite\r\n\r\n"); err != nil {
		t.Fatalf("Error io.WriteString() = %v", err)
	}

	resp, err := http.ReadResponse(bufio.NewReader(st.conn), nil)
	if err != nil {
		t.Fatalf("Error http.ReadResponse() = %v", err)
	}

	if got, want := resp.StatusCode, 200; got != want {
		t.Errorf("status: %v; want %v", got, want)
	}
	if got, want := resp.Header.Get("request-host"), st.backendHost; got != want {
		t.Errorf("request-host: %v; want %v", got, want)
	}
}

// TestH1H1RequestTrailer tests request trailer part is forwarded to
// backend.
func TestH1H1RequestTrailer(t *testing.T) {
	st := newServerTester(nil, t, func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 4096)
		for {
			_, err := r.Body.Read(buf)
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("r.Body.Read() = %v", err)
			}
		}
		if got, want := r.Trailer.Get("foo"), "bar"; got != want {
			t.Errorf("r.Trailer.Get(foo): %v; want %v", got, want)
		}
	})
	defer st.Close()

	res, err := st.http1(requestParam{
		name: "TestH1H1RequestTrailer",
		body: []byte("1"),
		trailer: []hpack.HeaderField{
			pair("foo", "bar"),
		},
	})
	if err != nil {
		t.Fatalf("Error st.http1() = %v", err)
	}
	if got, want := res.status, 200; got != want {
		t.Errorf("res.status: %v; want %v", got, want)
	}
}

// TestH1H1HeaderFieldBufferPath tests that request with request path
// larger than configured buffer size is rejected.
func TestH1H1HeaderFieldBufferPath(t *testing.T) {
	// The value 100 is chosen so that sum of header fields bytes
	// does not exceed it.  We use > 100 bytes URI to exceed this
	// limit.
	st := newServerTester([]string{"--header-field-buffer=100"}, t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("execution path should not be here")
	})
	defer st.Close()

	res, err := st.http1(requestParam{
		name: "TestH1H1HeaderFieldBufferPath",
		path: "/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaabbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	})
	if err != nil {
		t.Fatalf("Error st.http1() = %v", err)
	}
	if got, want := res.status, 431; got != want {
		t.Errorf("status: %v; want %v", got, want)
	}
}

// TestH1H1HeaderFieldBuffer tests that request with header fields
// larger than configured buffer size is rejected.
func TestH1H1HeaderFieldBuffer(t *testing.T) {
	st := newServerTester([]string{"--header-field-buffer=10"}, t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("execution path should not be here")
	})
	defer st.Close()

	res, err := st.http1(requestParam{
		name: "TestH1H1HeaderFieldBuffer",
	})
	if err != nil {
		t.Fatalf("Error st.http1() = %v", err)
	}
	if got, want := res.status, 431; got != want {
		t.Errorf("status: %v; want %v", got, want)
	}
}

// TestH1H1HeaderFields tests that request with header fields more
// than configured number is rejected.
func TestH1H1HeaderFields(t *testing.T) {
	st := newServerTester([]string{"--max-header-fields=1"}, t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("execution path should not be here")
	})
	defer st.Close()

	res, err := st.http1(requestParam{
		name: "TestH1H1HeaderFields",
		header: []hpack.HeaderField{
			// Add extra header field to ensure that
			// header field limit exceeds
			pair("Connection", "close"),
		},
	})
	if err != nil {
		t.Fatalf("Error st.http1() = %v", err)
	}
	if got, want := res.status, 431; got != want {
		t.Errorf("status: %v; want %v", got, want)
	}
}

// TestH1H1Websocket tests that HTTP Upgrade to WebSocket works.
func TestH1H1Websocket(t *testing.T) {
	st := newServerTesterHandler(nil, t, websocket.Handler(func(ws *websocket.Conn) {
		io.Copy(ws, ws)
	}))
	defer st.Close()

	content := []byte("hello world")
	res, err := st.websocket(requestParam{
		name: "TestH1H1Websocket",
		body: content,
	})
	if err != nil {
		t.Fatalf("Error st.websocket() = %v", err)
	}
	if got, want := res.body, content; !bytes.Equal(got, want) {
		t.Errorf("echo: %q; want %q", got, want)
	}
}

// TestH1H1ReqPhaseSetHeader tests mruby request phase hook
// modifies request header fields.
func TestH1H1ReqPhaseSetHeader(t *testing.T) {
	st := newServerTester([]string{"--mruby-file=" + testDir + "/req-set-header.rb"}, t, func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.Header.Get("User-Agent"), "mruby"; got != want {
			t.Errorf("User-Agent = %v; want %v", got, want)
		}
	})
	defer st.Close()

	res, err := st.http1(requestParam{
		name: "TestH1H1ReqPhaseSetHeader",
	})
	if err != nil {
		t.Fatalf("Error st.http1() = %v", err)
	}

	if got, want := res.status, 200; got != want {
		t.Errorf("status = %v; want %v", got, want)
	}
}

// TestH1H1ReqPhaseReturn tests mruby request phase hook returns
// custom response.
func TestH1H1ReqPhaseReturn(t *testing.T) {
	st := newServerTester([]string{"--mruby-file=" + testDir + "/req-return.rb"}, t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("request should not be forwarded")
	})
	defer st.Close()

	res, err := st.http1(requestParam{
		name: "TestH1H1ReqPhaseReturn",
	})
	if err != nil {
		t.Fatalf("Error st.http1() = %v", err)
	}

	if got, want := res.status, 404; got != want {
		t.Errorf("status = %v; want %v", got, want)
	}

	hdtests := []struct {
		k, v string
	}{
		{"content-length", "20"},
		{"from", "mruby"},
	}
	for _, tt := range hdtests {
		if got, want := res.header.Get(tt.k), tt.v; got != want {
			t.Errorf("%v = %v; want %v", tt.k, got, want)
		}
	}

	if got, want := string(res.body), "Hello World from req"; got != want {
		t.Errorf("body = %v; want %v", got, want)
	}
}

// TestH1H1RespPhaseSetHeader tests mruby response phase hook modifies
// response header fields.
func TestH1H1RespPhaseSetHeader(t *testing.T) {
	st := newServerTester([]string{"--mruby-file=" + testDir + "/resp-set-header.rb"}, t, noopHandler)
	defer st.Close()

	res, err := st.http1(requestParam{
		name: "TestH1H1RespPhaseSetHeader",
	})
	if err != nil {
		t.Fatalf("Error st.http1() = %v", err)
	}

	if got, want := res.status, 200; got != want {
		t.Errorf("status = %v; want %v", got, want)
	}

	if got, want := res.header.Get("alpha"), "bravo"; got != want {
		t.Errorf("alpha = %v; want %v", got, want)
	}
}

// TestH1H1RespPhaseReturn tests mruby response phase hook returns
// custom response.
func TestH1H1RespPhaseReturn(t *testing.T) {
	st := newServerTester([]string{"--mruby-file=" + testDir + "/resp-return.rb"}, t, noopHandler)
	defer st.Close()

	res, err := st.http1(requestParam{
		name: "TestH1H1RespPhaseReturn",
	})
	if err != nil {
		t.Fatalf("Error st.http1() = %v", err)
	}

	if got, want := res.status, 404; got != want {
		t.Errorf("status = %v; want %v", got, want)
	}

	hdtests := []struct {
		k, v string
	}{
		{"content-length", "21"},
		{"from", "mruby"},
	}
	for _, tt := range hdtests {
		if got, want := res.header.Get(tt.k), tt.v; got != want {
			t.Errorf("%v = %v; want %v", tt.k, got, want)
		}
	}

	if got, want := string(res.body), "Hello World from resp"; got != want {
		t.Errorf("body = %v; want %v", got, want)
	}
}

// // TestH1H2ConnectFailure tests that server handles the situation that
// // connection attempt to HTTP/2 backend failed.
// func TestH1H2ConnectFailure(t *testing.T) {
// 	st := newServerTester([]string{"--http2-bridge"}, t, noopHandler)
// 	defer st.Close()

// 	// simulate backend connect attempt failure
// 	st.ts.Close()

// 	res, err := st.http1(requestParam{
// 		name: "TestH1H2ConnectFailure",
// 	})
// 	if err != nil {
// 		t.Fatalf("Error st.http1() = %v", err)
// 	}
// 	want := 503
// 	if got := res.status; got != want {
// 		t.Errorf("status: %v; want %v", got, want)
// 	}
// }

// TestH1H2NoHost tests that server rejects request without Host
// header field for HTTP/2 backend.
func TestH1H2NoHost(t *testing.T) {
	st := newServerTester([]string{"--http2-bridge"}, t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("server should not forward bad request")
	})
	defer st.Close()

	// without Host header field, we expect 400 response
	if _, err := io.WriteString(st.conn, "GET / HTTP/1.1\r\nTest-Case: TestH1H2NoHost\r\n\r\n"); err != nil {
		t.Fatalf("Error io.WriteString() = %v", err)
	}

	resp, err := http.ReadResponse(bufio.NewReader(st.conn), nil)
	if err != nil {
		t.Fatalf("Error http.ReadResponse() = %v", err)
	}

	want := 400
	if got := resp.StatusCode; got != want {
		t.Errorf("status: %v; want %v", got, want)
	}
}

// TestH1H2HTTP10 tests that server can accept HTTP/1.0 request
// without Host header field
func TestH1H2HTTP10(t *testing.T) {
	st := newServerTester([]string{"--http2-bridge"}, t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("request-host", r.Host)
	})
	defer st.Close()

	if _, err := io.WriteString(st.conn, "GET / HTTP/1.0\r\nTest-Case: TestH1H2HTTP10\r\n\r\n"); err != nil {
		t.Fatalf("Error io.WriteString() = %v", err)
	}

	resp, err := http.ReadResponse(bufio.NewReader(st.conn), nil)
	if err != nil {
		t.Fatalf("Error http.ReadResponse() = %v", err)
	}

	if got, want := resp.StatusCode, 200; got != want {
		t.Errorf("status: %v; want %v", got, want)
	}
	if got, want := resp.Header.Get("request-host"), st.backendHost; got != want {
		t.Errorf("request-host: %v; want %v", got, want)
	}
}

// TestH1H2HTTP10NoHostRewrite tests that server generates host header
// field using actual backend server even if --no-http-rewrite is
// used.
func TestH1H2HTTP10NoHostRewrite(t *testing.T) {
	st := newServerTester([]string{"--http2-bridge"}, t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("request-host", r.Host)
	})
	defer st.Close()

	if _, err := io.WriteString(st.conn, "GET / HTTP/1.0\r\nTest-Case: TestH1H2HTTP10NoHostRewrite\r\n\r\n"); err != nil {
		t.Fatalf("Error io.WriteString() = %v", err)
	}

	resp, err := http.ReadResponse(bufio.NewReader(st.conn), nil)
	if err != nil {
		t.Fatalf("Error http.ReadResponse() = %v", err)
	}

	if got, want := resp.StatusCode, 200; got != want {
		t.Errorf("status: %v; want %v", got, want)
	}
	if got, want := resp.Header.Get("request-host"), st.backendHost; got != want {
		t.Errorf("request-host: %v; want %v", got, want)
	}
}

// TestH1H2CrumbleCookie tests that Cookies are crumbled and assembled
// when forwarding to HTTP/2 backend link.  go-nghttp2 server
// concatenates crumbled Cookies automatically, so this test is not
// much effective now.
func TestH1H2CrumbleCookie(t *testing.T) {
	st := newServerTester([]string{"--http2-bridge"}, t, func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.Header.Get("Cookie"), "alpha; bravo; charlie"; got != want {
			t.Errorf("Cookie: %v; want %v", got, want)
		}
	})
	defer st.Close()

	res, err := st.http1(requestParam{
		name: "TestH1H2CrumbleCookie",
		header: []hpack.HeaderField{
			pair("Cookie", "alpha; bravo; charlie"),
		},
	})
	if err != nil {
		t.Fatalf("Error st.http1() = %v", err)
	}
	if got, want := res.status, 200; got != want {
		t.Errorf("status: %v; want %v", got, want)
	}
}

// TestH1H2GenerateVia tests that server generates Via header field to and
// from backend server.
func TestH1H2GenerateVia(t *testing.T) {
	st := newServerTester([]string{"--http2-bridge"}, t, func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.Header.Get("Via"), "1.1 nghttpx"; got != want {
			t.Errorf("Via: %v; want %v", got, want)
		}
	})
	defer st.Close()

	res, err := st.http1(requestParam{
		name: "TestH1H2GenerateVia",
	})
	if err != nil {
		t.Fatalf("Error st.http1() = %v", err)
	}
	if got, want := res.header.Get("Via"), "2 nghttpx"; got != want {
		t.Errorf("Via: %v; want %v", got, want)
	}
}

// TestH1H2AppendVia tests that server adds value to existing Via
// header field to and from backend server.
func TestH1H2AppendVia(t *testing.T) {
	st := newServerTester([]string{"--http2-bridge"}, t, func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.Header.Get("Via"), "foo, 1.1 nghttpx"; got != want {
			t.Errorf("Via: %v; want %v", got, want)
		}
		w.Header().Add("Via", "bar")
	})
	defer st.Close()

	res, err := st.http1(requestParam{
		name: "TestH1H2AppendVia",
		header: []hpack.HeaderField{
			pair("via", "foo"),
		},
	})
	if err != nil {
		t.Fatalf("Error st.http1() = %v", err)
	}
	if got, want := res.header.Get("Via"), "bar, 2 nghttpx"; got != want {
		t.Errorf("Via: %v; want %v", got, want)
	}
}

// TestH1H2NoVia tests that server does not add value to existing Via
// header field to and from backend server.
func TestH1H2NoVia(t *testing.T) {
	st := newServerTester([]string{"--http2-bridge", "--no-via"}, t, func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.Header.Get("Via"), "foo"; got != want {
			t.Errorf("Via: %v; want %v", got, want)
		}
		w.Header().Add("Via", "bar")
	})
	defer st.Close()

	res, err := st.http1(requestParam{
		name: "TestH1H2NoVia",
		header: []hpack.HeaderField{
			pair("via", "foo"),
		},
	})
	if err != nil {
		t.Fatalf("Error st.http1() = %v", err)
	}
	if got, want := res.header.Get("Via"), "bar"; got != want {
		t.Errorf("Via: %v; want %v", got, want)
	}
}

// TestH1H2ReqPhaseReturn tests mruby request phase hook returns
// custom response.
func TestH1H2ReqPhaseReturn(t *testing.T) {
	st := newServerTester([]string{"--http2-bridge", "--mruby-file=" + testDir + "/req-return.rb"}, t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("request should not be forwarded")
	})
	defer st.Close()

	res, err := st.http1(requestParam{
		name: "TestH1H2ReqPhaseReturn",
	})
	if err != nil {
		t.Fatalf("Error st.http1() = %v", err)
	}

	if got, want := res.status, 404; got != want {
		t.Errorf("status = %v; want %v", got, want)
	}

	hdtests := []struct {
		k, v string
	}{
		{"content-length", "20"},
		{"from", "mruby"},
	}
	for _, tt := range hdtests {
		if got, want := res.header.Get(tt.k), tt.v; got != want {
			t.Errorf("%v = %v; want %v", tt.k, got, want)
		}
	}

	if got, want := string(res.body), "Hello World from req"; got != want {
		t.Errorf("body = %v; want %v", got, want)
	}
}

// TestH1H2RespPhaseReturn tests mruby response phase hook returns
// custom response.
func TestH1H2RespPhaseReturn(t *testing.T) {
	st := newServerTester([]string{"--http2-bridge", "--mruby-file=" + testDir + "/resp-return.rb"}, t, noopHandler)
	defer st.Close()

	res, err := st.http1(requestParam{
		name: "TestH1H2RespPhaseReturn",
	})
	if err != nil {
		t.Fatalf("Error st.http1() = %v", err)
	}

	if got, want := res.status, 404; got != want {
		t.Errorf("status = %v; want %v", got, want)
	}

	hdtests := []struct {
		k, v string
	}{
		{"content-length", "21"},
		{"from", "mruby"},
	}
	for _, tt := range hdtests {
		if got, want := res.header.Get(tt.k), tt.v; got != want {
			t.Errorf("%v = %v; want %v", tt.k, got, want)
		}
	}

	if got, want := string(res.body), "Hello World from resp"; got != want {
		t.Errorf("body = %v; want %v", got, want)
	}
}

// TestH1APIBackendconfig exercise backendconfig API endpoint routine
// for successful case.
func TestH1APIBackendconfig(t *testing.T) {
	st := newServerTesterConnectPort([]string{"-f127.0.0.1,3010;api;no-tls"}, t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("request should not be forwarded")
	}, 3010)
	defer st.Close()

	res, err := st.http1(requestParam{
		name:   "TestH1APIBackendconfig",
		path:   "/api/v1beta1/backendconfig",
		method: "PUT",
		body: []byte(`# comment
backend=127.0.0.1,3011

`),
	})
	if err != nil {
		t.Fatalf("Error st.http1() = %v", err)
	}
	if got, want := res.status, 200; got != want {
		t.Errorf("res.status: %v; want %v", got, want)
	}

	var apiResp APIResponse
	err = json.Unmarshal(res.body, &apiResp)
	if err != nil {
		t.Fatalf("Error unmarshaling API response: %v", err)
	}
	if got, want := apiResp.Status, "Success"; got != want {
		t.Errorf("apiResp.Status: %v; want %v", got, want)
	}
	if got, want := apiResp.Code, 200; got != want {
		t.Errorf("apiResp.Status: %v; want %v", got, want)
	}
}

// TestH1APIBackendconfigQuery exercise backendconfig API endpoint
// routine with query.
func TestH1APIBackendconfigQuery(t *testing.T) {
	st := newServerTesterConnectPort([]string{"-f127.0.0.1,3010;api;no-tls"}, t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("request should not be forwarded")
	}, 3010)
	defer st.Close()

	res, err := st.http1(requestParam{
		name:   "TestH1APIBackendconfigQuery",
		path:   "/api/v1beta1/backendconfig?foo=bar",
		method: "PUT",
		body: []byte(`# comment
backend=127.0.0.1,3011

`),
	})
	if err != nil {
		t.Fatalf("Error st.http1() = %v", err)
	}
	if got, want := res.status, 200; got != want {
		t.Errorf("res.status: %v; want %v", got, want)
	}

	var apiResp APIResponse
	err = json.Unmarshal(res.body, &apiResp)
	if err != nil {
		t.Fatalf("Error unmarshaling API response: %v", err)
	}
	if got, want := apiResp.Status, "Success"; got != want {
		t.Errorf("apiResp.Status: %v; want %v", got, want)
	}
	if got, want := apiResp.Code, 200; got != want {
		t.Errorf("apiResp.Status: %v; want %v", got, want)
	}
}

// TestH1APIBackendconfigBadMethod exercise backendconfig API endpoint
// routine with bad method.
func TestH1APIBackendconfigBadMethod(t *testing.T) {
	st := newServerTesterConnectPort([]string{"-f127.0.0.1,3010;api;no-tls"}, t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("request should not be forwarded")
	}, 3010)
	defer st.Close()

	res, err := st.http1(requestParam{
		name:   "TestH1APIBackendconfigBadMethod",
		path:   "/api/v1beta1/backendconfig",
		method: "GET",
		body: []byte(`# comment
backend=127.0.0.1,3011

`),
	})
	if err != nil {
		t.Fatalf("Error st.http1() = %v", err)
	}
	if got, want := res.status, 405; got != want {
		t.Errorf("res.status: %v; want %v", got, want)
	}

	var apiResp APIResponse
	err = json.Unmarshal(res.body, &apiResp)
	if err != nil {
		t.Fatalf("Error unmarshaling API response: %v", err)
	}
	if got, want := apiResp.Status, "Failure"; got != want {
		t.Errorf("apiResp.Status: %v; want %v", got, want)
	}
	if got, want := apiResp.Code, 405; got != want {
		t.Errorf("apiResp.Status: %v; want %v", got, want)
	}
}

// TestH1APINotFound exercise backendconfig API endpoint routine when
// API endpoint is not found.
func TestH1APINotFound(t *testing.T) {
	st := newServerTesterConnectPort([]string{"-f127.0.0.1,3010;api;no-tls"}, t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("request should not be forwarded")
	}, 3010)
	defer st.Close()

	res, err := st.http1(requestParam{
		name:   "TestH1APINotFound",
		path:   "/api/notfound",
		method: "GET",
		body: []byte(`# comment
backend=127.0.0.1,3011

`),
	})
	if err != nil {
		t.Fatalf("Error st.http1() = %v", err)
	}
	if got, want := res.status, 404; got != want {
		t.Errorf("res.status: %v; want %v", got, want)
	}

	var apiResp APIResponse
	err = json.Unmarshal(res.body, &apiResp)
	if err != nil {
		t.Fatalf("Error unmarshaling API response: %v", err)
	}
	if got, want := apiResp.Status, "Failure"; got != want {
		t.Errorf("apiResp.Status: %v; want %v", got, want)
	}
	if got, want := apiResp.Code, 404; got != want {
		t.Errorf("apiResp.Status: %v; want %v", got, want)
	}
}
