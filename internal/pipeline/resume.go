package pipeline

import (
	"context"
	"errors"
)

func (r *runner) resume(ctx context.Context) (*runner, error) {
	return nil, errors.New("resume is not implemented")
}
