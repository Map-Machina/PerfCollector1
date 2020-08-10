package channel

import (
	"context"
	"errors"
	"reflect"
)

var (
	ErrDone            = errors.New("shutting down")
	ErrUnexpectedClose = errors.New("closed")
	ErrChannelBusy     = errors.New("channel busy")
	ErrInvalidType     = errors.New("type assertion")
)

// isChanInterface return true is c is a chan interface{} type.
func isChanInterface(c interface{}) bool {
	if c == nil {
		return false
	}
	rt := reflect.TypeOf(c)
	if rt.Kind() != reflect.Chan {
		return false
	}

	return rt.Elem().Kind() == reflect.Interface
}

func WriteNB(ctx context.Context, c interface{}, payload interface{}) error {
	cases := []reflect.SelectCase{{
		Dir:  reflect.SelectRecv,
		Chan: reflect.ValueOf(ctx.Done()),
	}, {
		Dir:  reflect.SelectSend,
		Chan: reflect.ValueOf(c),
		Send: reflect.ValueOf(payload),
	}, {
		Dir: reflect.SelectDefault,
	}}

	chosen, _, _ := reflect.Select(cases)
	switch chosen {
	case 0:
		return ErrDone //ctx.Err()
	case 1:
		return nil
	case 2:
		return ErrChannelBusy
	default:
		panic("unreachable")
	}
}

func Write(ctx context.Context, c interface{}, payload interface{}) error {
	cases := []reflect.SelectCase{{
		Dir:  reflect.SelectRecv,
		Chan: reflect.ValueOf(ctx.Done()),
	}, {
		Dir:  reflect.SelectSend,
		Chan: reflect.ValueOf(c),
		Send: reflect.ValueOf(payload),
	}}

	chosen, _, _ := reflect.Select(cases)
	switch chosen {
	case 0:
		return ErrDone //ctx.Err()
	case 1:
		return nil
	default:
		panic("unreachable")
	}
}

func Read(ctx context.Context, c interface{}) (interface{}, error) {
	cases := []reflect.SelectCase{{
		Dir:  reflect.SelectRecv,
		Chan: reflect.ValueOf(ctx.Done()),
	}, {
		Dir:  reflect.SelectRecv,
		Chan: reflect.ValueOf(c),
	}}

	chosen, recv, recvOK := reflect.Select(cases)
	switch chosen {
	case 0:
		return nil, ErrDone //ctx.Err()
	case 1:
		if !recvOK {
			return nil, ErrUnexpectedClose
		}
		return recv.Interface(), nil
	default:
		panic("unreachable")
	}
}
