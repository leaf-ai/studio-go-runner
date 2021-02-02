// Copyright 2021 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package wrapper

import (
	"github.com/leaf-ai/studio-go-runner/internal/defense"
	"github.com/leaf-ai/studio-go-runner/internal/request"

	"github.com/jjeffery/kv"
)

type Wrapper interface {
	WrapRequest(r *request.Request) (encrypted string, err kv.Error)
	UnwrapRequest(encrypted string) (r *request.Request, err kv.Error)
	Envelope(r *request.Request) (e *defense.Envelope, err kv.Error)
	Request(e *defense.Envelope) (r *request.Request, err kv.Error)
}
