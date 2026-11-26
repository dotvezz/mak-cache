package valkey

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/dotvezz/mak-cache/config"
	"github.com/dotvezz/mak-cache/storage"

	"github.com/prometheus/client_golang/prometheus"

	vkey "github.com/valkey-io/valkey-go"
)

var now = time.Now
var newClient = vkey.NewClient

type Provider[C any, T interface {
	*C
	storage.Storable
}] struct {
	client  vkey.Client
	timeout time.Duration

	hits       prometheus.Counter
	gets       prometheus.Counter
	getErrors  prometheus.Counter
	getSeconds prometheus.Histogram

	sets       prometheus.Counter
	setErrors  prometheus.Counter
	setSeconds prometheus.Histogram
}

func (p *Provider[C, T]) Get(ctx context.Context, key string) (_ T, err error) {
	if p.getSeconds != nil {
		start := now()
		defer func() {
			p.getSeconds.Observe(now().Sub(start).Seconds())

			p.gets.Inc()

			if err == nil {
				p.hits.Inc()
			} else if !errors.Is(err, storage.ErrNotFound) {
				// storage.ErrNotFound is a normal miss, not counted as an error
				p.getErrors.Inc()
			}
		}()
	}

	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	var value C
	v := T(&value)
	var bs []byte
	bs, err = p.client.Do(ctx, p.client.B().Get().Key(key).Build()).AsBytes()

	if err != nil {
		if vkey.IsValkeyNil(err) {
			err = storage.ErrNotFound
			return v, err
		}
		err = fmt.Errorf("failed to fetch key %q: %w", key, err)
		return v, err
	}

	err = v.Unmarshal(bytes.NewReader(bs))
	if err != nil {
		err = fmt.Errorf("failed to unmarshal key %q: %w", key, err)
		return v, err
	}

	return v, err
}

func (p *Provider[C, T]) Set(ctx context.Context, key string, value T) (err error) {
	if p.setSeconds != nil {
		start := now()
		defer func() {
			p.setSeconds.Observe(now().Sub(start).Seconds())
			p.sets.Inc()
			if err != nil {
				p.setErrors.Inc()
			}
		}()
	}

	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	bs := bytes.NewBuffer(make([]byte, 0, 1024))
	err = value.MarshalTo(bs)
	if err != nil {
		return fmt.Errorf("failed to marshal value %q: %w", key, err)
	}

	err = p.client.Do(ctx,
		p.client.
			B().
			Set().
			Key(key).
			Value(vkey.BinaryString(bs.Bytes())).
			Ex(value.EvictAt().Sub(now())).
			Build(),
	).Error()
	if err != nil {
		err = fmt.Errorf("failed to set value %q: %w", key, err)
		return err
	}

	return err
}

func (p *Provider[C, T]) Update(ctx context.Context, k string, v T) error {
	return p.Set(ctx, k, v)
}

func NewProvider[C any, T interface {
	*C
	storage.Storable
}](cfg config.ValkeyConfig) (*Provider[C, T], error) {
	client, err := newClient(vkey.ClientOption{
		InitAddress: []string{cfg.Address},
	})

	return &Provider[C, T]{
		client:  client,
		timeout: cfg.ResolveStorageTimeout(),
	}, err
}
