package forgejo

import (
	"context"
	"net/http"
)

type User struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
}

type Users struct{ client *Client }

func (u *Users) Me(ctx context.Context) (*User, error) {
	var user User
	_, err := u.client.Do(ctx, http.MethodGet, "user", nil, &user)
	if err != nil {
		return nil, err
	}
	return &user, nil
}
