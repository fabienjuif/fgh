package main

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/fabienjuif/fgh/auth"
	"github.com/fabienjuif/fgh/forgejo"
	"github.com/fabienjuif/fgh/remote"
)

type authError struct{ err error }

func (e *authError) Error() string { return e.err.Error() }
func (e *authError) Unwrap() error { return e.err }

func exitCode(err error) int {
	var authentication *authError
	if errors.As(err, &authentication) {
		return 4
	}
	return 1
}

func newAuthCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "auth", Short: "Authenticate with Forgejo", RunE: groupRunE}
	var statusHost, loginHost string
	status := &cobra.Command{
		Use: "status", Short: "Show authentication status", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			host, err := authHost(cmd, statusHost)
			if err != nil {
				return err
			}
			credential, err := auth.Token(cmd.Context(), host)
			if err != nil {
				return &authError{err}
			}
			user, err := forgejo.New("https://"+host, credential.Token).Users.Me(cmd.Context())
			if err != nil {
				return &authError{err}
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s\n  Logged in to %s account %s (%s)\n", host, host, user.Login, credential.Source)
			return err
		},
	}
	status.Flags().Bool("help", false, "Help for status")
	status.Flags().StringVarP(&statusHost, "hostname", "h", "", "Forgejo hostname")
	var withToken bool
	login := &cobra.Command{
		Use: "login", Short: "Authenticate with a token", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !withToken {
				return errors.New("interactive login is not implemented; use --with-token")
			}
			host, err := authHost(cmd, loginHost)
			if err != nil {
				return err
			}
			data, err := io.ReadAll(io.LimitReader(cmd.InOrStdin(), 1<<20))
			if err != nil {
				return errors.New("cannot read token")
			}
			token := strings.TrimSpace(string(data))
			if token == "" || strings.ContainsAny(token, "\r\n") {
				return errors.New("expected one token on stdin")
			}
			if _, err = forgejo.New("https://"+host, token).Users.Me(cmd.Context()); err != nil {
				return &authError{err}
			}
			return auth.Save(host, token)
		},
	}
	login.Flags().Bool("help", false, "Help for login")
	login.Flags().BoolVar(&withToken, "with-token", false, "Read token from standard input")
	login.Flags().StringVarP(&loginHost, "hostname", "h", "", "Forgejo hostname")
	cmd.AddCommand(status, login)
	return cmd
}

func authHost(cmd *cobra.Command, hostname string) (string, error) {
	if hostname != "" {
		if strings.ContainsAny(hostname, "/\r\n ") {
			return "", errors.New("invalid hostname")
		}
		return hostname, nil
	}
	override, err := cmd.Flags().GetString("repo")
	if err != nil {
		return "", err
	}
	if override == "" {
		return remote.Host(cmd.Context(), "")
	}
	repository, err := remote.Resolve(cmd.Context(), "", override)
	if err != nil {
		return "", err
	}
	return repository.Host, nil
}
