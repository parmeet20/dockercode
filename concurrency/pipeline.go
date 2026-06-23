package concurrency

import (
	"context"
)

type Chunk struct {
	Data string
	Err  error
	Done bool
}

func Pipe(ctx context.Context, in <-chan Chunk, out chan<- Chunk) {
	defer close(out)
	for {
		select {
		case <-ctx.Done():
			return
		case chunk, ok := <-in:
			if !ok {
				return
			}
			select {
			case out <- chunk:
			case <-ctx.Done():
				return
			}
			if chunk.Err != nil || chunk.Done {
				return
			}
		}
	}
}
