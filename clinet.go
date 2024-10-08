package wrpc

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/gogf/gf/v2/container/gmap"
	"github.com/gogf/gf/v2/encoding/gjson"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/util/gconv"
	"github.com/gorilla/websocket"
)

type Client struct {
	id         int
	conn       *websocket.Conn
	counter    int64
	writeMutex sync.Mutex
	response   *gmap.IntAnyMap
	handlerMap map[string]*handlerFuncInfo
	datas      map[string]any
	datasMutex sync.Mutex
}

func NewClient(socket *websocket.Conn) *Client {
	r := &Client{
		id:         generateClientId(),
		conn:       socket,
		counter:    0,
		response:   gmap.NewIntAnyMap(true),
		handlerMap: map[string]*handlerFuncInfo{},
		datas:      map[string]any{},
	}
	return r
}

var clientCounter int64

func generateClientId() int {
	return int(atomic.AddInt64(&clientCounter, 1))
}

func (c *Client) GetId() int {
	return c.id
}

func (c *Client) writeResponse(msg Message) error {
	c.writeMutex.Lock()
	defer c.writeMutex.Unlock()
	return c.conn.WriteJSON(msg)
}

func (c *Client) generateRequestId() int {
	return int(atomic.AddInt64(&c.counter, 1))
}

// Request send request and wait response
func (c *Client) Request(ctx context.Context, req RequestData) (interface{}, error) {
	if req.Timeout == 0 {
		req.Timeout = DefaultTimeout
	}
	reqId := c.generateRequestId()

	ctx, cancel := context.WithTimeout(ctx, req.Timeout)
	defer cancel()

	ch := make(chan Message)
	defer close(ch)
	c.response.Set(reqId, ch)
	defer c.response.Remove(reqId)

	err := c.writeResponse(Message{
		Id:      reqId,
		Kind:    RequestKind,
		Command: req.Command,
		Data:    req.Data,
	})
	if err != nil {
		return nil, err
	}

	select {
	case rsp := <-ch:
		if rsp.Code != 0 {
			return nil, errors.New(gconv.String(rsp.Message))
		}
		return rsp.Data, nil
	case <-ctx.Done():
		return nil, gerror.New("request timeout")
	}
}

func (c *Client) RequestAndUnmarshal(ctx context.Context, req RequestData, pointer interface{}, paramKeyToAttrMap ...map[string]string) error {
	rsp, err := c.Request(ctx, req)
	if err != nil {
		return err
	}
	return gconv.Struct(rsp, pointer, paramKeyToAttrMap...)
}

func (c *Client) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			return err
		}
		// if tpe != websocket.TextMessage {
		// 	continue
		// }
		if len(data) == 0 {
			continue
		}
		var msg *Message
		err = gjson.DecodeTo(data, &msg)
		if err != nil {
			continue
		}

		go c.handleMessage(ctx, msg)
	}
}

func (c *Client) handleMessage(ctx context.Context, msg *Message) (err error) {
	defer func() {
		if err != nil {
			c.writeResponse(Message{
				Id:      msg.Id,
				Kind:    ResponseKind,
				Message: err.Error(),
			})
		}
	}()

	switch msg.Kind {
	case RequestKind:
		err = c.handleCall(ctx, msg)
	case ResponseKind:
		err = c.handleResponse(ctx, msg)
	default:
		err = errors.New("not support kind")
	}

	return
}

type clientKey struct{}

func (c *Client) handleCall(ctx context.Context, msg *Message) error {
	ctx = context.WithValue(ctx, clientKey{}, c)
	data, err := c.call(ctx, msg.Command, msg.Data)
	if err != nil {
		// ignore this error result
		return c.writeResponse(Message{
			Id:      msg.Id,
			Kind:    ResponseKind,
			Command: msg.Command,
			Code:    1,
			Message: err.Error(),
			Data:    nil,
		})
	}
	// success call handle
	return c.writeResponse(Message{
		Id:      msg.Id,
		Kind:    ResponseKind,
		Command: msg.Command,
		Code:    0,
		Message: "ok",
		Data:    data,
	})
}

func (c *Client) handleResponse(ctx context.Context, msg *Message) error {
	ch := c.response.Remove(int(msg.Id))
	if ch != nil {
		ch.(chan Message) <- *msg
	}

	return nil
}

func (c *Client) Close() error {
	return c.conn.Close()
}

func (c *Client) SetData(key string, data any) {
	c.datasMutex.Lock()
	defer c.datasMutex.Unlock()

	c.datas[key] = data
}

func (c *Client) GetData(key string) any {
	c.datasMutex.Lock()
	defer c.datasMutex.Unlock()

	return c.datas[key]
}

func ClientFronCtx(ctx context.Context) *Client {
	v := ctx.Value(clientKey{})
	if client, ok := v.(*Client); ok {
		return client
	}
	return nil
}
