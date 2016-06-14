package httpjson

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"reflect"
	"strings"
	"time"

	"code.uber.internal/infra/memtsdb/services/m3dbnode/serve"
	"code.uber.internal/infra/memtsdb/services/m3dbnode/serve/tchannelthrift"
	"code.uber.internal/infra/memtsdb/services/m3dbnode/serve/tchannelthrift/thrift/gen-go/rpc"
	"code.uber.internal/infra/memtsdb/storage"
	xerrors "code.uber.internal/infra/memtsdb/x/errors"

	"github.com/uber/tchannel-go/thrift"
)

const (
	defaultReadTimeout    = 10 * time.Second
	defaultWriteTimeout   = 10 * time.Second
	defaultRequestTimeout = 60 * time.Second
)

var (
	errRequestMustBePost  = xerrors.NewInvalidParamsError(errors.New("request must be POST"))
	errInvalidRequestBody = xerrors.NewInvalidParamsError(errors.New("request contains an invalid request body"))
	errEncodeResponseBody = errors.New("failed to encode response body")
)

type server struct {
	address string
	db      storage.Database
	opts    ServerOptions
}

// ServerOptions is a set of server options
type ServerOptions interface {
	// ReadTimeout sets the readTimeout and returns a new ServerOptions
	ReadTimeout(value time.Duration) ServerOptions

	// GetReadTimeout returns the readTimeout
	GetReadTimeout() time.Duration

	// WriteTimeout sets the writeTimeout and returns a new ServerOptions
	WriteTimeout(value time.Duration) ServerOptions

	// GetWriteTimeout returns the writeTimeout
	GetWriteTimeout() time.Duration

	// RequestTimeout sets the requestTimeout and returns a new ServerOptions
	RequestTimeout(value time.Duration) ServerOptions

	// GetRequestTimeout returns the requestTimeout
	GetRequestTimeout() time.Duration
}

type serverOptions struct {
	readTimeout    time.Duration
	writeTimeout   time.Duration
	requestTimeout time.Duration
}

// NewServerOptions creates a new set of server options with defaults
func NewServerOptions() ServerOptions {
	return &serverOptions{
		readTimeout:    defaultReadTimeout,
		writeTimeout:   defaultWriteTimeout,
		requestTimeout: defaultRequestTimeout,
	}
}

func (o *serverOptions) ReadTimeout(value time.Duration) ServerOptions {
	opts := *o
	opts.readTimeout = value
	return &opts
}

func (o *serverOptions) GetReadTimeout() time.Duration {
	return o.readTimeout
}

func (o *serverOptions) WriteTimeout(value time.Duration) ServerOptions {
	opts := *o
	opts.writeTimeout = value
	return &opts
}

func (o *serverOptions) GetWriteTimeout() time.Duration {
	return o.writeTimeout
}

func (o *serverOptions) RequestTimeout(value time.Duration) ServerOptions {
	opts := *o
	opts.requestTimeout = value
	return &opts
}

func (o *serverOptions) GetRequestTimeout() time.Duration {
	return o.requestTimeout
}

// NewServer creates a TChannel Thrift network service
func NewServer(
	db storage.Database,
	address string,
	opts ServerOptions,
) serve.NetworkService {
	if opts == nil {
		opts = NewServerOptions()
	}
	return &server{
		address: address,
		db:      db,
		opts:    opts,
	}
}

func (s *server) ListenAndServe() (serve.Close, error) {
	mux := http.NewServeMux()
	if err := registerHandlers(mux, tchannelthrift.NewService(s.db), s.opts); err != nil {
		return nil, err
	}

	listener, err := net.Listen("tcp", s.address)
	if err != nil {
		return nil, err
	}

	server := http.Server{
		Handler:      mux,
		ReadTimeout:  s.opts.GetReadTimeout(),
		WriteTimeout: s.opts.GetWriteTimeout(),
	}

	go func() {
		server.Serve(listener)
	}()

	return func() {
		listener.Close()
	}, nil
}

func defaultDuration(value time.Duration, defaultValue time.Duration) time.Duration {
	if value == time.Duration(0) {
		return defaultValue
	}
	return value
}

type respSuccess struct {
}
type respErrorResult struct {
	Error respError `json:"error"`
}
type respError struct {
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

func registerHandlers(mux *http.ServeMux, service rpc.TChanNode, opts ServerOptions) error {
	v := reflect.ValueOf(service)
	t := v.Type()
	for i := 0; i < t.NumMethod(); i++ {
		method := t.Method(i)
		// Ensure this method is of either:
		// - methodName(RequestObject) error
		// - methodName(RequestObject) (ResultObject, error)
		if method.Type.NumIn() != 3 || !(method.Type.NumOut() == 1 || method.Type.NumOut() == 2) {
			continue
		}

		obj := method.Type.In(0)
		context := method.Type.In(1)
		reqIn := method.Type.In(2)
		var resultOut, resultErr reflect.Type
		if method.Type.NumOut() == 1 {
			resultErr = method.Type.Out(0)
		} else {
			resultOut = method.Type.Out(0)
			resultErr = method.Type.Out(1)
		}

		serviceInterfaceType := reflect.TypeOf((*rpc.TChanNode)(nil)).Elem()
		if !obj.Implements(serviceInterfaceType) {
			continue
		}

		contextInterfaceType := reflect.TypeOf((*thrift.Context)(nil)).Elem()
		if context.Kind() != reflect.Interface || !context.Implements(contextInterfaceType) {
			continue
		}

		if reqIn.Kind() != reflect.Ptr || reqIn.Elem().Kind() != reflect.Struct {
			continue
		}

		if method.Type.NumOut() == 2 {
			if resultOut.Kind() != reflect.Ptr || resultOut.Elem().Kind() != reflect.Struct {
				continue
			}
		}

		errInterfaceType := reflect.TypeOf((*error)(nil)).Elem()
		if resultErr.Kind() != reflect.Interface || !resultErr.Implements(errInterfaceType) {
			continue
		}

		name := strings.ToLower(method.Name)
		mux.HandleFunc(fmt.Sprintf("/%s", name), func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if strings.ToLower(r.Method) != "post" {
				writeError(w, errRequestMustBePost)
				return
			}

			in := reflect.New(reqIn.Elem()).Interface()
			defer r.Body.Close()
			if err := json.NewDecoder(r.Body).Decode(in); err != nil {
				writeError(w, errInvalidRequestBody)
				return
			}

			svc := reflect.ValueOf(service)
			callContext, _ := thrift.NewContext(opts.GetRequestTimeout())
			ctx := reflect.ValueOf(callContext)
			ret := method.Func.Call([]reflect.Value{svc, ctx, reflect.ValueOf(in)})
			if method.Type.NumOut() == 1 {
				// Deal with error case
				if !ret[0].IsNil() {
					writeError(w, ret[0].Interface())
					return
				}
				json.NewEncoder(w).Encode(&respSuccess{})
				return
			}

			// Deal with error case
			if !ret[1].IsNil() {
				writeError(w, ret[1].Interface())
				return
			}

			buff := bytes.NewBuffer(nil)
			if err := json.NewEncoder(buff).Encode(ret[0].Interface()); err != nil {
				writeError(w, errEncodeResponseBody)
				return
			}

			w.Write(buff.Bytes())
		})
	}
	return nil
}

func writeError(w http.ResponseWriter, errValue interface{}) {
	result := respErrorResult{respError{}}
	if value, ok := errValue.(error); ok {
		result.Error.Message = value.Error()
	} else if value, ok := errValue.(fmt.Stringer); ok {
		result.Error.Message = value.String()
	}
	result.Error.Data = errValue

	buff := bytes.NewBuffer(nil)
	if err := json.NewEncoder(buff).Encode(&result); err != nil {
		// Not a JSON returnable error
		w.WriteHeader(http.StatusInternalServerError)
		result.Error.Message = fmt.Sprintf("%v", errValue)
		result.Error.Data = nil
		json.NewEncoder(w).Encode(&result)
		return
	}

	if value, ok := errValue.(error); ok && xerrors.IsInvalidParams(value) {
		w.WriteHeader(http.StatusBadRequest)
	} else {
		w.WriteHeader(http.StatusInternalServerError)
	}
	w.Write(buff.Bytes())
}