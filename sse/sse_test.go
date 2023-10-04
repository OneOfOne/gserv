package sse_test

import (
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.oneofone.dev/gserv"
	"go.oneofone.dev/gserv/sse"
)

func TestSSE(t *testing.T) {
	t.Skip()
	if testing.Short() {
		t.Skip("not supported in short mode")
	}

	srv := gserv.New()
	if testing.Verbose() {
		srv.Use(gserv.LogRequests(true))
	}

	ts := httptest.NewServer(srv)
	defer ts.Close()

	done := make(chan struct{}, 1)
	sr := sse.NewRouter()

	srv.GET("/sse/:id", func(ctx *gserv.Context) gserv.Response {
		log.Println("new connection", ctx.Req.RemoteAddr)
		sr.Handle(ctx.Param("id"), 10, ctx)
		return nil
	})

	srv.GET("/send/:id", func(ctx *gserv.Context) gserv.Response {
		log.Println("new connection", ctx.Req.RemoteAddr)
		sr.Send(ctx.Param("id"), time.Now().String(), "", ctx.Query("m"))
		ctx.WriteHeader(http.StatusNoContent)
		return nil
	})

	srv.GET("/close", func(ctx *gserv.Context) gserv.Response {
		close(done)
		return nil
	})

	srv.GET("/", func(ctx *gserv.Context) gserv.Response {
		ctx.Write([]byte(page))
		return nil
	})

	log.Printf("listening on: %s", ts.URL)

	<-done
}

const page = `
<!DOCTYPE html>
<html>
<head>
	<title>Test</title>
</head>
<body>
	<script type="text/javascript">
		const es = new EventSource('/sse/user1');
		// Create a callback for when a new message is received.
		es.onmessage = function(e) {
			console.log(e);
			document.body.innerHTML += '<li>' + e.data + '</li>';
		};
		setTimeout(async () => await fetch('/send/user1?m=hello'), 1000);
	</script>
	<a href="/close">close</a>
	<br>
	<ul>
</body>
</html>
`
