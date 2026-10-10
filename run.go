package main

import (
	"context"
)

func run(ctx context.Context) error {
	app, err := newApp(ctx)
	if err != nil {
		return err
	}

	defer app.Close()

	return app.Run(ctx)
}
