package wrpc

import (
	"time"
)

const DefaultTimeout = 60 * time.Second

type MessageKind int

const RequestKind MessageKind = 0
const ResponseKind MessageKind = 1

type Message struct {
	Id      int         `json:"id"`
	Kind    MessageKind `json:"kind"`
	Command string      `json:"command"`
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

type RequestData struct {
	Command string        `json:"command"`
	Data    interface{}   `json:"data"`
	Timeout time.Duration `json:"timeout"`
}
