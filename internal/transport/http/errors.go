package http

import "errors"

var (
	errNoRecords            = errors.New("no records provided")
	errStreamingUnsupported = errors.New("streaming unsupported")
	errInvalidApplication   = errors.New("application name is required")
)
