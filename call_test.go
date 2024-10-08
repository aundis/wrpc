package wrpc

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/os/gctx"
	"github.com/gogf/gf/v2/util/gconv"
	"github.com/gorilla/websocket"
)

func init() {

}

var upgrader = websocket.Upgrader{} // use default options
func TestCall(t *testing.T) {
	go func() {
		// 服务端
		s := g.Server()
		s.BindHandler("/ws", func(r *ghttp.Request) {
			conn, err := upgrader.Upgrade(r.Response.Writer, r.Request, nil)
			if err != nil {
				t.Error("WebSocket upgrade error:", err)
				return
			}
			defer conn.Close()

			ctx := gctx.New()
			client := NewClient(conn)
			client.SetData("foo", "bar")
			client.MustBind("add", addHandlerfunc)
			client.Start(ctx)
		})
		s.SetPort(8199)
		s.Run()
	}()

	time.Sleep(1 * time.Second)

	// 客户端
	conn, _, err := websocket.DefaultDialer.Dial("ws://127.0.0.1:8199/ws", nil)
	// t.Log("dial rsp:", rsp)
	if err != nil {
		t.Error("ws dial error:", err)
		return
	}

	ctx := gctx.New()
	client := NewClient(conn)
	go func() { client.Start(ctx) }()
	result, err := client.Request(ctx, RequestData{
		Command: "add",
		Data: &addReq{
			A: 10,
			B: 20,
		},
		// Timeout: 10 * time.Second,
	})
	if err != nil {
		t.Error("request error:", err)
		return
	}
	if gconv.Int(result) != 30 {
		t.Errorf("except 30 but got %v", result)
		return
	}

	// 校验规则测试
	_, err = client.Request(ctx, RequestData{
		Command: "add",
		Data: &addReq{
			A: 1,
			B: 20,
		},
		// Timeout: 10 * time.Second,
	})
	if err == nil {
		t.Error("except error but got nil")
		return
	}
	if !strings.Contains(err.Error(), "must be equal or greater than 5") {
		t.Errorf("got err %v", err)
		return
	}
}

type addReq struct {
	A int `json:"a" v:"min:5"`
	B int `json:"b"`
}

func addHandlerfunc(ctx context.Context, req *addReq) (int, error) {
	client := ClientFronCtx(ctx)
	data := client.GetData("foo")
	if data != "bar" {
		return 0, gerror.Newf("except bar but got %v", data)
	}

	return req.A + req.B, nil
}

func TestUnmarshalNil(t *testing.T) {
	var a map[string]string
	err := gconv.Struct(nil, &a)
	if err != nil {
		t.Error(err)
		return
	}
}
