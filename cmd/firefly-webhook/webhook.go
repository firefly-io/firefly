package main

import (
	"os"

	"k8s.io/component-base/cli"
	_ "k8s.io/component-base/logs/json/register" // for JSON log format registration

	"github.com/carlory/firefly/cmd/firefly-webhook/app"
)

func main() {
	cmd := app.NewWebhookCommand()
	code := cli.Run(cmd)
	os.Exit(code)
}
