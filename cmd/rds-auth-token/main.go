/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
// Command rds-auth-token mints an AWS RDS/Aurora IAM authentication token and
// writes it to a file.
//
// It exists so that the chart's `psql`-based init jobs (create-database,
// create-importers) can authenticate to an IAM-enabled RDS instance: `psql`
// cannot generate a token itself, and the trustd image ships no AWS tooling.
// The chart runs this binary as an init container, from the operator image,
// and the job container then reads the token into PGPASSWORD.
//
// Credentials are resolved through the standard AWS chain, so this works for
// every CCO mode: manual/STS (projected service-account token exchanged via
// AssumeRoleWithWebIdentity) as well as mint/passthrough/default (static keys
// from the CCO secret).
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/rds/auth"
)

// timeout bounds the whole operation. Minting a token needs at most one STS
// round-trip; the signing itself is local.
const timeout = 30 * time.Second

func main() {
	out := flag.String("out", envOr("RDS_AUTH_TOKEN_FILE", "/var/run/rds-auth-token/token"),
		"file to write the token to, or \"-\" for stdout")
	flag.Parse()

	if err := run(*out); err != nil {
		fmt.Fprintf(os.Stderr, "rds-auth-token: %v\n", err)
		os.Exit(1)
	}
}

func run(out string) error {
	// The connection parameters are taken from the same PG* variables the
	// chart already renders for the psql container, so the token can never be
	// minted for a different endpoint or user than the one psql connects as.
	host, err := required("PGHOST")
	if err != nil {
		return err
	}
	user, err := required("PGUSER")
	if err != nil {
		return err
	}
	port := envOr("PGPORT", "5432")

	// AWS_REGION is set by the chart from .Values.ccoRds.region.
	region, err := required("AWS_REGION")
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return fmt.Errorf("loading AWS configuration: %w", err)
	}

	endpoint := fmt.Sprintf("%s:%s", host, port)
	token, err := auth.BuildAuthToken(ctx, endpoint, region, user, cfg.Credentials)
	if err != nil {
		return fmt.Errorf("building RDS auth token for %s@%s: %w", user, endpoint, err)
	}

	if out == "-" {
		fmt.Println(token)
		return nil
	}

	// The token is a credential: keep it readable only by the pod's user.
	if err := os.WriteFile(out, []byte(token), 0o600); err != nil {
		return fmt.Errorf("writing token to %s: %w", out, err)
	}

	// Never log the token itself.
	fmt.Fprintf(os.Stderr, "wrote RDS auth token for %s@%s to %s\n", user, endpoint, out)
	return nil
}

func required(name string) (string, error) {
	value := os.Getenv(name)
	if value == "" {
		return "", fmt.Errorf("%s must be set", name)
	}
	return value, nil
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
