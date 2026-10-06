package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/darkodemic/gpwebcam/internal/record"
	"github.com/darkodemic/gpwebcam/internal/tray"
)

const recordUsage = `usage: gpwebcam record [start|stop]

Starts or stops a recording in the running "gpwebcam run", as the tray
menu does. The camera's video goes into a Matroska file in ~/Videos/gpwebcam
(or the folder given to "gpwebcam run" with -record-dir), as the camera
sends it, without audio. Without arguments it says whether gpwebcam
records.
`

// recordWait bounds how long "record start" waits for the file to open: an
// idle camera with camera mode demand starts first, which takes about 5 s.
const recordWait = 20 * time.Second

func cmdRecord(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("record", flag.ContinueOnError)
	fs.Usage = func() { fmt.Fprint(fs.Output(), recordUsage) }
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 1 {
		fs.Usage()
		return errors.New("record takes start, stop or nothing")
	}
	client, _, err := controlClient()
	if err != nil {
		return err
	}
	switch fs.Arg(0) {
	case "":
		st, err := getStatus(client)
		if err != nil {
			return err
		}
		printRecording(stdout, st)
		return nil
	case "start":
		if _, err := call(client, http.MethodPost, "/v1/record/start", nil); err != nil {
			return err
		}
		deadline := time.Now().Add(recordWait)
		for {
			st, err := getStatus(client)
			if err != nil {
				return err
			}
			switch {
			case !st.Recording && st.Error != "":
				return fmt.Errorf("recording stopped: %s", st.Error)
			case !st.Recording:
				return errors.New("recording stopped; see: journalctl --user -u gpwebcam")
			case st.File != "":
				fmt.Fprintf(stdout, "Recording to %s\n", st.File)
				return nil
			case time.Now().After(deadline):
				fmt.Fprintln(stdout, "Recording starts as soon as the camera streams.")
				return nil
			}
			time.Sleep(300 * time.Millisecond)
		}
	case "stop":
		var saved savedRecording
		if _, err := call(client, http.MethodPost, "/v1/record/stop", &saved); err != nil {
			return err
		}
		if saved.File == "" {
			fmt.Fprintln(stdout, "Recording stopped before the camera streamed; no file was written.")
			return nil
		}
		fmt.Fprintf(stdout, "Saved %s (%s, %s)\n", saved.File,
			tray.Clock(time.Duration(saved.Seconds*float64(time.Second))), record.SizeText(uint64(saved.Bytes)))
		if saved.Dropped > 0 {
			fmt.Fprintf(stdout, "The disk was too slow for %d datagrams; the file has gaps.\n", saved.Dropped)
		}
		return nil
	default:
		fs.Usage()
		return fmt.Errorf("unknown record command %q", fs.Arg(0))
	}
}

func printRecording(w io.Writer, st controlStatus) {
	switch {
	case st.Recording && st.File != "":
		fmt.Fprintf(w, "Recording to %s for %s\n", st.File, tray.Clock(time.Since(st.Since)))
	case st.Recording:
		fmt.Fprintln(w, "Recording starts as soon as the camera streams.")
	case st.Error != "":
		fmt.Fprintf(w, "Not recording. The last recording stopped: %s\n", st.Error)
	default:
		fmt.Fprintln(w, "Not recording.")
	}
}

func getStatus(client *http.Client) (controlStatus, error) {
	var st controlStatus
	_, err := call(client, http.MethodGet, "/v1/status", &st)
	return st, err
}

// call sends a request to the control API and decodes the reply into out;
// a reply with an error field becomes an error.
func call(client *http.Client, method, path string, out any) (int, error) {
	req, err := http.NewRequest(method, controlHost+path, nil)
	if err != nil {
		return 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("talk to gpwebcam run: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return resp.StatusCode, err
	}
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(body, &e) == nil && e.Error != "" {
			return resp.StatusCode, errors.New(e.Error)
		}
		return resp.StatusCode, fmt.Errorf("gpwebcam run answered %s", resp.Status)
	}
	if out != nil {
		if err := json.Unmarshal(body, out); err != nil {
			return resp.StatusCode, fmt.Errorf("reply of gpwebcam run: %w", err)
		}
	}
	return resp.StatusCode, nil
}
