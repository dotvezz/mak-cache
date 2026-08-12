package valkey

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/dotvezz/caddy-cache/config"
	"github.com/dotvezz/caddy-cache/storage"

	vkey "github.com/valkey-io/valkey-go"
)

type Provider[C any, T interface {
	*C
	storage.Storable
}] struct {
	client vkey.Client
	now    func() time.Time
}

func (p Provider[C, T]) Get(ctx context.Context, key string) (T, error) {
	var value C
	v := T(&value)
	bs, err := p.client.Do(ctx, p.client.B().Get().Key(key).Build()).AsBytes()

	if err != nil {
		if vkey.IsValkeyNil(err) {
			return v, storage.ErrNotFound
		}
		return v, fmt.Errorf("failed to fetch key %q: %w", key, err)
	}

	err = v.Unmarshal(bytes.NewReader(bs))
	if err != nil {
		return v, fmt.Errorf("failed to unmarshal key %q: %w", key, err)
	}

	return v, nil
}

func (p Provider[C, T]) Set(ctx context.Context, key string, value T) error {
	bs := bytes.NewBuffer(make([]byte, 0, 1024))
	err := value.MarshalTo(bs)
	if err != nil {
		return fmt.Errorf("failed to marshal value %q: %w", key, err)
	}

	err = p.client.Do(ctx, p.client.B().Set().Key(key).Value(vkey.BinaryString(bs.Bytes())).Build()).Error()
	if err != nil {
		return fmt.Errorf("failed to set value %q: %w", key, err)
	}

	return nil
}

func (p Provider[C, T]) Update(ctx context.Context, k string, v T) error {
	return p.Set(ctx, k, v)
}

func NewProvider[C any, T interface {
	*C
	storage.Storable
}](cfg config.ValkeyConfig) (*Provider[C, T], error) {
	client, err := vkey.NewClient(vkey.ClientOption{
		InitAddress: []string{cfg.Address},
	})

	return &Provider[C, T]{
		client: client,
		now:    time.Now,
	}, err
}
