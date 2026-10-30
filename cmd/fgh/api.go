package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/spf13/cobra"

	"github.com/fabienjuif/fgh/auth"
	"github.com/fabienjuif/fgh/forgejo"
	"github.com/fabienjuif/fgh/remote"
)

type apiOptions struct {
	method   string
	raw      []string
	typed    []string
	headers  []string
	jq       string
	paginate bool
	input    string
	include  bool
	silent   bool
}

func newAPICmd() *cobra.Command {
	var o apiOptions
	cmd := &cobra.Command{
		Use:   "api <endpoint>",
		Short: "Make an authenticated Forgejo API request",
		Long: "Make a request to /api/v1/<endpoint>. {owner} and {repo} are filled from the remote or -R.\n" +
			"With -f/-F and no --method the request is a POST; on GET the fields become the query string.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error { return runAPI(cmd, args[0], &o) },
	}
	f := cmd.Flags()
	f.StringVarP(&o.method, "method", "X", "", "The HTTP method for the request (default \"GET\")")
	f.StringArrayVarP(&o.raw, "raw-field", "f", nil, "Add a string parameter in key=value format")
	f.StringArrayVarP(&o.typed, "field", "F", nil, "Add a typed parameter in key=value format (@file or @- reads a value)")
	f.StringArrayVarP(&o.headers, "header", "H", nil, "Add a HTTP request header in key:value format")
	f.StringVarP(&o.jq, "jq", "q", "", "Query to select values from the response using jq syntax")
	f.BoolVar(&o.paginate, "paginate", false, "Make additional HTTP requests to fetch all pages of results")
	f.StringVar(&o.input, "input", "", "The file to use as the request body (- for stdin)")
	f.BoolVarP(&o.include, "include", "i", false, "Include the HTTP response status line and headers in the output")
	f.BoolVar(&o.silent, "silent", false, "Do not print the response body")
	return cmd
}

var placeholder = regexp.MustCompile(`\{(owner|repo)\}`)

func runAPI(cmd *cobra.Command, endpoint string, o *apiOptions) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	override, _ := cmd.Flags().GetString("repo")
	var host string
	if placeholder.MatchString(endpoint) {
		repository, err := remote.Resolve(ctx, "", override)
		if err != nil {
			return err
		}
		host = repository.Host
		endpoint = placeholder.ReplaceAllStringFunc(endpoint, func(m string) string {
			if m == "{owner}" {
				return repository.Owner
			}
			return repository.Repo
		})
	} else {
		var err error
		if host, err = authHost(cmd, ""); err != nil {
			return err
		}
	}
	method := strings.ToUpper(o.method)
	hasFields := len(o.raw)+len(o.typed) > 0
	if method == "" {
		method = http.MethodGet
		if hasFields || o.input != "" {
			method = http.MethodPost
		}
	}
	stdin := cmd.InOrStdin()
	fields, err := buildFields(o, stdin)
	if err != nil {
		return err
	}
	var body io.Reader
	header := http.Header{}
	switch {
	case o.input != "":
		if body, err = openInput(o.input, stdin); err != nil {
			return err
		}
		// With --input, fields can only travel in the query string, as in gh.
		endpoint, err = addQuery(endpoint, fields)
		if err != nil {
			return err
		}
		// Forgejo rejects a body without a Content-Type (422).
		header.Set("Content-Type", "application/json")
	case method == http.MethodGet || method == http.MethodHead:
		if endpoint, err = addQuery(endpoint, fields); err != nil {
			return err
		}
	case hasFields:
		data, marshalErr := json.Marshal(fields)
		if marshalErr != nil {
			return marshalErr
		}
		body = bytes.NewReader(data)
		header.Set("Content-Type", "application/json")
	}
	for _, h := range o.headers {
		name, value, ok := strings.Cut(h, ":")
		if !ok || strings.TrimSpace(name) == "" {
			return fmt.Errorf("invalid header %q: expected key:value", h)
		}
		header.Set(strings.TrimSpace(name), strings.TrimSpace(value))
	}
	var code *gojq.Code
	if o.jq != "" {
		query, parseErr := gojq.Parse(o.jq)
		if parseErr != nil {
			return fmt.Errorf("invalid jq expression: %w", parseErr)
		}
		if code, err = gojq.Compile(query); err != nil {
			return fmt.Errorf("invalid jq expression: %w", err)
		}
	}
	credential, err := auth.Token(ctx, host)
	if err != nil {
		return &authError{err}
	}
	client := forgejo.New("https://"+host, credential.Token)
	out, errOut := cmd.OutOrStdout(), cmd.ErrOrStderr()
	path := endpoint
	for {
		res, err := client.Raw(ctx, method, path, header, body)
		if err != nil {
			return err
		}
		data, readErr := io.ReadAll(res.Body)
		_ = res.Body.Close()
		if readErr != nil {
			return readErr
		}
		if o.include {
			_, _ = fmt.Fprintf(out, "%s %s\r\n", res.Proto, res.Status)
			_ = res.Header.Write(out)
			_, _ = fmt.Fprint(out, "\r\n")
		}
		if res.StatusCode < 200 || res.StatusCode >= 300 {
			_, _ = errOut.Write(data)
			if len(data) > 0 && data[len(data)-1] != '\n' {
				_, _ = fmt.Fprintln(errOut)
			}
			return fmt.Errorf("HTTP %d", res.StatusCode)
		}
		if !o.silent {
			if err = printBody(out, data, code); err != nil {
				return err
			}
		}
		next := ""
		if o.paginate {
			next = forgejo.NextLink(res.Header)
		}
		if next == "" {
			return nil
		}
		path = next
		body = nil
	}
}

func printBody(out io.Writer, data []byte, code *gojq.Code) error {
	if code == nil {
		_, err := out.Write(data)
		return err
	}
	var value any
	if len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&value); err != nil {
		return fmt.Errorf("--jq needs a JSON response: %w", err)
	}
	iter := code.Run(normalize(value))
	for {
		v, ok := iter.Next()
		if !ok {
			return nil
		}
		if err, isErr := v.(error); isErr {
			return fmt.Errorf("jq: %w", err)
		}
		// gh prints strings raw and everything else as compact JSON.
		if s, isString := v.(string); isString {
			_, _ = fmt.Fprintln(out, s)
			continue
		}
		enc, err := json.Marshal(v)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(out, "%s\n", enc)
	}
}

// normalize turns json.Number into the int or float64 gojq expects.
func normalize(v any) any {
	switch x := v.(type) {
	case json.Number:
		if i, err := strconv.ParseInt(x.String(), 10, 0); err == nil {
			return int(i)
		}
		f, _ := x.Float64()
		return f
	case []any:
		for i := range x {
			x[i] = normalize(x[i])
		}
	case map[string]any:
		for k := range x {
			x[k] = normalize(x[k])
		}
	}
	return v
}

func openInput(name string, stdin io.Reader) (io.Reader, error) {
	if name == "-" {
		return stdin, nil
	}
	data, err := os.ReadFile(name)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

func addQuery(endpoint string, fields map[string]any) (string, error) {
	if len(fields) == 0 {
		return endpoint, nil
	}
	q := url.Values{}
	for k, v := range fields {
		switch x := v.(type) {
		case []any:
			for _, item := range x {
				q.Add(k, fmt.Sprint(item))
			}
		case map[string]any:
			return "", fmt.Errorf("nested parameter %q cannot be sent in a query string", k)
		case nil:
			q.Add(k, "")
		default:
			q.Add(k, fmt.Sprint(x))
		}
	}
	sep := "?"
	if strings.Contains(endpoint, "?") {
		sep = "&"
	}
	return endpoint + sep + q.Encode(), nil
}

func buildFields(o *apiOptions, stdin io.Reader) (map[string]any, error) {
	fields := map[string]any{}
	for _, kv := range o.raw {
		key, value, ok := strings.Cut(kv, "=")
		if !ok {
			return nil, fmt.Errorf("field %q requires a value separated by an '=' sign", kv)
		}
		if err := setField(fields, key, value); err != nil {
			return nil, err
		}
	}
	for _, kv := range o.typed {
		key, value, ok := strings.Cut(kv, "=")
		if !ok {
			return nil, fmt.Errorf("field %q requires a value separated by an '=' sign", kv)
		}
		var typed any = value
		switch {
		case value == "true":
			typed = true
		case value == "false":
			typed = false
		case value == "null":
			typed = nil
		case isInteger(value):
			n, _ := strconv.ParseInt(value, 10, 64)
			typed = n
		case strings.HasPrefix(value, "@"):
			var data []byte
			var err error
			if value == "@-" {
				data, err = io.ReadAll(stdin)
			} else {
				data, err = os.ReadFile(value[1:])
			}
			if err != nil {
				return nil, fmt.Errorf("reading value of %q: %w", key, err)
			}
			typed = string(data)
		}
		if err := setField(fields, key, typed); err != nil {
			return nil, err
		}
	}
	return fields, nil
}

func isInteger(s string) bool {
	_, err := strconv.ParseInt(s, 10, 64)
	return err == nil && !strings.HasPrefix(s, "+")
}

var bracket = regexp.MustCompile(`\[([^\]]*)\]`)

// setField handles gh's key syntax: "a[b]=1" nests, "a[]=1" appends to a list.
func setField(fields map[string]any, key string, value any) error {
	first, _, hasBracket := strings.Cut(key, "[")
	if !hasBracket {
		fields[key] = value
		return nil
	}
	rest := key[len(first):]
	var path []string
	for _, m := range bracket.FindAllStringSubmatch(rest, -1) {
		path = append(path, m[1])
	}
	if first == "" || len(path) == 0 || strings.Join(bracket.Split(rest, -1), "") != "" {
		return fmt.Errorf("invalid field name %q", key)
	}
	cur := fields
	name := first
	for i, p := range path {
		last := i == len(path)-1
		if p == "" {
			if !last {
				return fmt.Errorf("invalid field name %q: [] must come last", key)
			}
			list, _ := cur[name].([]any)
			cur[name] = append(list, value)
			return nil
		}
		child, ok := cur[name].(map[string]any)
		if !ok {
			if _, exists := cur[name]; exists {
				return errors.New("conflicting field " + key)
			}
			child = map[string]any{}
			cur[name] = child
		}
		cur, name = child, p
	}
	cur[name] = value
	return nil
}
