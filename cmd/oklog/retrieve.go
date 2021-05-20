package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/oklog/oklog/pkg/store"
	"github.com/pkg/errors"
)

func runRetrieve(args []string) error {
	flagset := flag.NewFlagSet("retrieve", flag.ExitOnError)
	var (
		storeAddr = flagset.String("store", "localhost:7650", "address of store instance to query")
		from      = flagset.String("from", "1h", "from, as RFC3339 timestamp or duration ago")
		to        = flagset.String("to", "now", "to, as RFC3339 timestamp or duration ago")
		withulid  = flagset.Bool("ulid", false, "include ULID prefix with each record")
		withtime  = flagset.Bool("time", false, "include time prefix with each record")
		verbose   = flagset.Bool("v", false, "verbose output to stderr")
	)
	flagset.Usage = usageFor(flagset, "oklog retrieve [flags]")
	if err := flagset.Parse(args); err != nil {
		return err
	}

	verbosePrintf := func(string, ...interface{}) {}
	if *verbose {
		verbosePrintf = func(format string, args ...interface{}) {
			fmt.Fprintf(os.Stderr, format, args...)
		}
	}

	_, hostport, _, _, err := parseAddr(*storeAddr, defaultAPIPort)
	if err != nil {
		return errors.Wrap(err, "couldn't parse -store")
	}

	fromDuration, durationErr := time.ParseDuration(*from)
	fromTime, timeErr := time.Parse(time.RFC3339Nano, *from)
	fromNow := strings.ToLower(*from) == "now"
	var fromStr string
	switch {
	case fromNow:
		fromStr = time.Now().Format(time.RFC3339)
	case durationErr == nil && timeErr != nil:
		fromStr = time.Now().Add(neg(fromDuration)).Format(time.RFC3339)
	case durationErr != nil && timeErr == nil:
		fromStr = fromTime.Format(time.RFC3339)
	default:
		return fmt.Errorf("couldn't parse -from (%q) as either duration or time", *from)
	}

	toDuration, durationErr := time.ParseDuration(*to)
	toTime, timeErr := time.Parse(time.RFC3339, *to)
	toNow := strings.ToLower(*to) == "now"
	var toStr string
	switch {
	case toNow:
		toStr = time.Now().Format(time.RFC3339)
	case durationErr == nil && timeErr != nil:
		toStr = time.Now().Add(neg(toDuration)).Format(time.RFC3339)
	case durationErr != nil && timeErr == nil:
		toStr = toTime.Format(time.RFC3339)
	default:
		return fmt.Errorf("couldn't parse -to (%q) as either duration or time", *to)
	}

	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf(
		"http://%s/store%s?from=%s&to=%s",
		hostport,
		store.APIPathDCSQuery,
		url.QueryEscape(fromStr),
		url.QueryEscape(toStr),
	), nil)
	if err != nil {
		return err
	}
	verbosePrintf("GET %s\n", req.URL.String())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		req.URL.RawQuery = "" // for pretty print
		return errors.Errorf("%s %s: %s", req.Method, req.URL.String(), resp.Status)
	}

	switch {
	case *withulid:
		io.Copy(os.Stdout, resp.Body)
	case *withtime:
		io.Copy(os.Stdout, parseTime(resp.Body))
	default:
		io.Copy(os.Stdout, strip(resp.Body))
	}

	return nil
}
