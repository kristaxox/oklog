package store

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"time"

	"github.com/pkg/errors"
	"storj.io/uplink"
)

const (
	uploadTimeout   = 15 * time.Second
	downloadTimeout = 15 * time.Second
)

var (
	_ Log          = (*fileLogDCS)(nil)
	_ TrashSegment = (fileTrashSegmentDCS{})
)

// fileLogDCS wraps fileLog and intercepts Purgeable to wrap fileTrashSegment
// with fileTrashSegmentDCS.
type fileLogDCS struct {
	*fileLog

	project *uplink.Project

	bucketName string
}

func (fl *fileLogDCS) Purgeable(oldestModTime time.Time) ([]TrashSegment, error) {
	trashSegments, err := fl.fileLog.Purgeable(oldestModTime)
	if err != nil {
		return trashSegments, err
	}

	var trashSegmentsDCS []TrashSegment

	for _, s := range trashSegments {
		trashSegmentsDCS = append(trashSegmentsDCS, fileTrashSegmentDCS{
			reporter:         fl.reporter,
			fileTrashSegment: s.(fileTrashSegment), // TODO(amwolff): check whether it's always true.
			project:          fl.project,
			bucketName:       fl.bucketName,
		})
	}

	return trashSegmentsDCS, nil
}

// Close closes only fileLogDCS's resources and won't close underlying fileLog.
func (fl *fileLogDCS) Close() error {
	return fl.project.Close()
}

// NewFileLogDCS returns initialized fileLogDCS.
func NewFileLogDCS(ctx context.Context, log Log, project *uplink.Project, bucketName string) (Log, error) {
	if _, err := project.EnsureBucket(ctx, bucketName); err != nil {
		return nil, errors.Wrap(err, "EnsureBucket")
	}
	return &fileLogDCS{
		fileLog:    log.(*fileLog), // TODO(amwolff): check whether it's always true.
		project:    project,
		bucketName: bucketName,
	}, nil
}

// fileTrashSegmentDCS wraps fileTrashSegment and moves segments to Storj DCS on
// Purge.
type fileTrashSegmentDCS struct {
	reporter EventReporter
	fileTrashSegment

	project *uplink.Project

	bucketName string
}

func abortUnlessCommitted(reporter EventReporter, file string, upload *uplink.Upload) {
	if err := upload.Abort(); err != nil {
		if errors.Is(err, uplink.ErrUploadDone) {
			reporter.ReportEvent(Event{
				Debug: true,
				File:  file,
				Msg:   "Upload already aborted/committed",
			})
			return
		}
		reporter.ReportEvent(Event{
			File:  file,
			Error: err,
			Msg:   "Could not abort upload",
		})
		return
	}
	reporter.ReportEvent(Event{
		File: file,
		Msg:  "Upload aborted",
	})
}

func (t fileTrashSegmentDCS) Purge() error {
	ctx, cancel := context.WithTimeout(context.Background(), uploadTimeout)
	defer cancel()

	key := basename(t.f.Name())

	upload, err := t.project.UploadObject(ctx, t.bucketName, fmt.Sprintf("%s.gz", key), nil)
	if err != nil {
		return errors.Wrap(err, "UploadObject")
	}
	defer abortUnlessCommitted(t.reporter, key, upload)

	w := gzip.NewWriter(upload)

	n, err := io.Copy(w, t.f)
	if err != nil {
		return errors.Wrap(err, "Copy")
	}

	if err := w.Close(); err != nil {
		return errors.Wrap(err, "Close")
	}

	t.reporter.ReportEvent(Event{
		Debug: true,
		Msg:   fmt.Sprintf("Uploaded %d bytes to DCS", n),
	})

	if err = upload.Commit(); err != nil {
		return errors.Wrap(err, "Commit")
	}

	return t.fileTrashSegment.Purge() // The segment is in DCS now, and we're safe to delete it.
}

func (fl *fileLogDCS) DCSReader(qp QueryParams) io.Reader {
	pr, pw := io.Pipe()

	go func() {
		defer pw.Close()

		ctx, cancel := context.WithTimeout(context.Background(), downloadTimeout)
		defer cancel()

		var totalBytes int64

		// TODO: sorting the segment files from the bucket, an equivalent to sorting in queryMatchingSegments()
		iterator := fl.project.ListObjects(ctx, fl.bucketName, nil)
		for iterator.Next() {
			low, high, err := parseFilename(iterator.Item().Key)
			if err != nil {
				fl.reportDCSWarning(err, iterator.Item().Key)
				continue
			}

			if !overlap(qp.From.ULID, qp.To.ULID, low, high) {
				continue
			}

			download, err := fl.project.DownloadObject(ctx, fl.bucketName, iterator.Item().Key, nil)
			if err != nil {
				fl.reportDCSError(err, iterator.Item().Key)
				continue
			}

			gzDownload, err := gzip.NewReader(download)
			if err != nil {
				download.Close()
				fl.reportDCSError(err, iterator.Item().Key)
				continue
			}

			n, err := io.Copy(pw, gzDownload)
			if err != nil {
				gzDownload.Close()
				download.Close()
				fl.reportDCSError(err, iterator.Item().Key)
				continue
			}

			gzDownload.Close()
			download.Close()

			totalBytes += n
		}

		fl.reporter.ReportEvent(Event{
			Debug: true,
			Op:    "DCSReader",
			Msg:   fmt.Sprintf("Downloaded %d bytes from DCS", totalBytes),
		})
	}()

	return pr
}

func (fl *fileLogDCS) reportDCSWarning(err error, filename string) {
	fl.reporter.ReportEvent(Event{
		Op:      "DCSReader",
		File:    fmt.Sprintf("sj://%s/%s", fl.bucketName, filename),
		Warning: err,
	})
}

func (fl *fileLogDCS) reportDCSError(err error, filename string) {
	fl.reporter.ReportEvent(Event{
		Op:    "DCSReader",
		File:  fmt.Sprintf("sj://%s/%s", fl.bucketName, filename),
		Error: err,
	})
}
