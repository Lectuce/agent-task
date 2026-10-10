package main

import (
	"context"
	"fmt"
)

func run(ctx context.Context) error {
	app, err := newApp(ctx)
	if err != nil {
		fmt.Println(err.Error())
	}

	defer app.Close()

	return app.Run(ctx)
}
