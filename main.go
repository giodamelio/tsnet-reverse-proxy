package main

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	flag "github.com/spf13/pflag"
	"tailscale.com/tsnet"
)

type cliArgs struct {
	// The url to advertise on your Tailnet
	fromURL *url.URL

	// The url to forward traffic to
	toURL *url.URL
}

func printHelp() {
	executablePath, _ := os.Executable()
	executable := path.Base(executablePath)
	// TODO: update the help
	fmt.Printf("Usage: %s <tailscale hostname> [tailscale port] to <forward hostname> <forward port>\n", executable)
	fmt.Println("Examples:")
	fmt.Println("  # Forward traffic from Tailnet hello:8080 to localhost:8080")
	fmt.Printf("  $ %s hello to localhost 8080\n", executable)
}

func parseArgs() *cliArgs {
	flag.Parse()
	args := flag.Args()
	argsLength := len(args)

	if argsLength == 2 {
		// TODO: handle these errors
		fromURL, _ := url.Parse(args[0])
		toURL, _ := url.Parse(args[1])

		// Use port 80 as the default port if none is specified
		if fromURL.Port() == "" {
			fromURL.Host = fromURL.Host + ":80"
		}
		if toURL.Port() == "" {
			toURL.Host = toURL.Host + ":80"
		}

		return &cliArgs{
			fromURL: fromURL,
			toURL:   toURL,
		}
	}

	printHelp()
	os.Exit(1)

	return nil
}

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	// Ensure our args are reasonable
	args := parseArgs()

	tailscaleServer := &tsnet.Server{
		Hostname: "hello",
		// Hide Tailscale Logs
		Logf: func(format string, args ...any) {
			// log.Debug().Str("component", "tailscale").Msgf(format, args...)
		},
	}
	defer tailscaleServer.Close()

	tailscaleListener, err := tailscaleServer.Listen("tcp", ":"+args.fromURL.Port())
	if err != nil {
		log.Fatal().Err(err)
	}
	defer tailscaleListener.Close()
	log.Info().Msg("Tailscale server started")

	// Create a Tailscale API Client to allow fetching user data
	tailscaleClient, err := tailscaleServer.LocalClient()
	if err != nil {
		log.Fatal().Err(err)
	}

	// Forward traffic from the Tailscale Server on
	proxy := httputil.NewSingleHostReverseProxy(args.toURL)
	originalDirector := proxy.Director
	proxy.Director = func(r *http.Request) {
		originalDirector(r)

		who, err := tailscaleClient.WhoIs(r.Context(), r.RemoteAddr)
		if err != nil {
			return
		}

		r.Header.Add("x-tailscale-id", who.UserProfile.ID.String())
		r.Header.Add("x-tailscale-username", who.UserProfile.LoginName)
		r.Header.Add("x-tailscale-displayname", who.UserProfile.DisplayName)
		r.Header.Add("x-tailscale-computed-node", who.Node.ComputedName)
	}

	// Start the proxy
	log.Fatal().Err(http.Serve(tailscaleListener, proxy))
}
