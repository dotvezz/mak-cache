package valkey

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
	"unsafe"

	"github.com/dotvezz/mak-cache/cache"
	"github.com/dotvezz/mak-cache/config"
	"github.com/dotvezz/mak-cache/storage"
	vkey "github.com/valkey-io/valkey-go"
)

func newBuilder() vkey.Builder {
	var b vkey.Builder
	pks := (*uint16)(unsafe.Pointer(&b))
	*pks = 32768 // 0x8000: unassigned key slot flag in valkey-go
	return b
}

type mockClient struct {
	vkey.Client
	doFunc func(ctx context.Context, cmd vkey.Completed) vkey.ValkeyResult
}

func (m *mockClient) Do(ctx context.Context, cmd vkey.Completed) vkey.ValkeyResult {
	if m.doFunc != nil {
		return m.doFunc(ctx, cmd)
	}
	return vkey.ValkeyResult{}
}

func (m *mockClient) B() vkey.Builder {
	return newBuilder()
}

func newMockResult(bs []byte, err error) vkey.ValkeyResult {
	var res vkey.ValkeyResult
	vRes := reflect.ValueOf(&res).Elem()
	if err != nil {
		pErr := unsafe.Pointer(vRes.Field(0).UnsafeAddr())
		*(*error)(pErr) = err
	}
	if len(bs) > 0 {
		vMsg := vRes.Field(1)
		pBytes := unsafe.Pointer(vMsg.Field(1).UnsafeAddr())
		*(*(*uint8))(pBytes) = &bs[0]

		pIntlen := unsafe.Pointer(vMsg.Field(3).UnsafeAddr())
		*(*int64)(pIntlen) = int64(len(bs))

		pTyp := unsafe.Pointer(vMsg.Field(4).UnsafeAddr())
		*(*uint8)(pTyp) = '$'
	}
	return res
}

func TestNewProvider(t *testing.T) {
	t.Run("successful provider creation", func(t *testing.T) {
		mock := &mockClient{}
		newClient = func(opt vkey.ClientOption) (vkey.Client, error) {
			return mock, nil
		}
		defer func() {
			newClient = vkey.NewClient
		}()

		p, err := NewProvider[cache.Entry, *cache.Entry](config.ValkeyConfig{Address: "127.0.0.1:6379"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p == nil {
			t.Fatal("expected non-nil provider")
		}
	})

	t.Run("error during client creation", func(t *testing.T) {
		expectedErr := errors.New("client creation error")
		newClient = func(opt vkey.ClientOption) (vkey.Client, error) {
			return nil, expectedErr
		}
		defer func() {
			newClient = vkey.NewClient
		}()

		_, err := NewProvider[cache.Entry, *cache.Entry](config.ValkeyConfig{Address: "invalid"})
		if !errors.Is(err, expectedErr) {
			t.Errorf("got error %v, want %v", err, expectedErr)
		}
	})
}

func TestProvider_Get(t *testing.T) {
	entry := &cache.Entry{
		Status: 200,
		Body:   []byte("test body"),
	}
	buf := bytes.NewBuffer(nil)
	_ = entry.MarshalTo(buf)
	entryBytes := buf.Bytes()

	t.Run("successful Get", func(t *testing.T) {
		mock := &mockClient{
			doFunc: func(ctx context.Context, cmd vkey.Completed) vkey.ValkeyResult {
				return newMockResult(entryBytes, nil)
			},
		}

		p := &Provider[cache.Entry, *cache.Entry]{client: mock}
		val, err := p.Get(context.Background(), "k1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if val.Status != 200 || string(val.Body) != "test body" {
			t.Errorf("got entry %+v, want status 200 body 'test body'", val)
		}
	})

	t.Run("key not found returns ErrNotFound", func(t *testing.T) {
		mock := &mockClient{
			doFunc: func(ctx context.Context, cmd vkey.Completed) vkey.ValkeyResult {
				return newMockResult(nil, vkey.Nil)
			},
		}

		p := &Provider[cache.Entry, *cache.Entry]{client: mock}
		_, err := p.Get(context.Background(), "k1")
		if !errors.Is(err, storage.ErrNotFound) {
			t.Errorf("got error %v, want %v", err, storage.ErrNotFound)
		}
	})

	t.Run("generic client error during Get", func(t *testing.T) {
		clientErr := errors.New("valkey cluster connection error")
		mock := &mockClient{
			doFunc: func(ctx context.Context, cmd vkey.Completed) vkey.ValkeyResult {
				return newMockResult(nil, clientErr)
			},
		}

		p := &Provider[cache.Entry, *cache.Entry]{client: mock}
		_, err := p.Get(context.Background(), "k1")
		if err == nil || !errors.Is(err, clientErr) {
			t.Errorf("expected error containing client error, got %v", err)
		}
	})

	t.Run("unmarshal error", func(t *testing.T) {
		corrupted := []byte("bad_corrupted_data")
		mock := &mockClient{
			doFunc: func(ctx context.Context, cmd vkey.Completed) vkey.ValkeyResult {
				return newMockResult(corrupted, nil)
			},
		}

		p := &Provider[cache.Entry, *cache.Entry]{client: mock}
		_, err := p.Get(context.Background(), "k1")
		if err == nil {
			t.Fatal("expected unmarshal error, got nil")
		}
	})
}

func TestProvider_SetAndUpdate(t *testing.T) {
	entry := &cache.Entry{
		Status: 200,
		Body:   []byte("test body"),
		Metadata: cache.Metadata{
			Expires: time.Now().Add(time.Hour),
		},
	}

	t.Run("successful Set and Update", func(t *testing.T) {
		doCalled := false
		mock := &mockClient{
			doFunc: func(ctx context.Context, cmd vkey.Completed) vkey.ValkeyResult {
				doCalled = true
				return newMockResult(nil, nil)
			},
		}

		p := &Provider[cache.Entry, *cache.Entry]{client: mock}
		err := p.Set(context.Background(), "k1", entry)
		if err != nil {
			t.Fatalf("unexpected Set error: %v", err)
		}
		if !doCalled {
			t.Error("expected client.Do to be called")
		}

		doCalled = false
		err = p.Update(context.Background(), "k1", entry)
		if err != nil {
			t.Fatalf("unexpected Update error: %v", err)
		}
		if !doCalled {
			t.Error("expected client.Do to be called for Update")
		}
	})

	t.Run("client error during Set", func(t *testing.T) {
		clientErr := errors.New("write timeout")
		mock := &mockClient{
			doFunc: func(ctx context.Context, cmd vkey.Completed) vkey.ValkeyResult {
				return newMockResult(nil, clientErr)
			},
		}

		p := &Provider[cache.Entry, *cache.Entry]{client: mock}
		err := p.Set(context.Background(), "k1", entry)
		if err == nil || !errors.Is(err, clientErr) {
			t.Errorf("expected Set error containing client error, got %v", err)
		}
	})
}
