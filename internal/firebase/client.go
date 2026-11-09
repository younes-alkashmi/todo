package firebase

import (
	"context"

	"cloud.google.com/go/firestore"
	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/auth"
	"google.golang.org/api/option"
)

type Client struct {
	App       *firebase.App
	Auth      *auth.Client
	Firestore *firestore.Client
}

func New(ctx context.Context, saPath, projectID string) (*Client, error) {
	opt := option.WithCredentialsFile(saPath)
	app, err := firebase.NewApp(ctx, &firebase.Config{ProjectID: projectID}, opt)
	if err != nil {
		return nil, err
	}
	authClient, err := app.Auth(ctx)
	if err != nil {
		return nil, err
	}
	fsClient, err := app.Firestore(ctx)
	if err != nil {
		return nil, err
	}
	return &Client{App: app, Auth: authClient, Firestore: fsClient}, nil
}

func (c *Client) Close() { c.Firestore.Close() }
