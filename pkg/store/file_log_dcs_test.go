package store

import (
	"compress/gzip"
	"fmt"
	"io"
	"testing"

	"github.com/oklog/oklog/pkg/fs"
	"storj.io/common/testcontext"
	"storj.io/storj/private/testplanet"
)

type testEventReporter struct {
	t *testing.T
}

// ReportEvent implements EventReporter.
func (r testEventReporter) ReportEvent(e Event) {
	r.t.Logf("%+v", e)
}

func Test_fileTrashSegmentDCS_Purge(t *testing.T) {
	testplanet.Run(t, testplanet.Config{
		SatelliteCount:   1,
		StorageNodeCount: 0, // uploaded/downloaded object will be an inline segment
		UplinkCount:      1,
	}, func(t *testing.T, ctx *testcontext.Context, planet *testplanet.Planet) {
		virtualFS := fs.NewVirtualFilesystem()

		const testfilename = "testfile"

		testfile, err := virtualFS.Create(testfilename)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		const testdata = "During the 1980s, the operating system Plan 9 from Bell Labs was developed extending the UNIX model."

		if _, err = testfile.Write([]byte(testdata)); err != nil {
			t.Fatalf("Write: %v", err)
		}

		project, err := planet.Uplinks[0].OpenProject(ctx, planet.Satellites[0])
		if err != nil {
			t.Fatalf("OpenProject: %v", err)
		}
		defer ctx.Check(project.Close)

		const bucketName = "hackathon"

		if _, err = project.EnsureBucket(ctx, bucketName); err != nil {
			t.Fatalf("EnsureBucket: %v", err)
		}

		segment := fileTrashSegmentDCS{
			reporter:         testEventReporter{t: t},
			fileTrashSegment: fileTrashSegment{virtualFS, testfile},
			project:          project,
			bucketName:       bucketName,
		}

		if err = segment.Purge(); err != nil {
			t.Fatalf("Purge: %v", err)
		}

		if virtualFS.Exists(testfilename) {
			t.Errorf("%s shouldn't exist at this point", testfilename)
		}

		download, err := project.DownloadObject(ctx, bucketName, fmt.Sprintf("%s.gz", testfilename), nil)
		if err != nil {
			t.Fatalf("DownloadObject: %v", err)
		}
		defer ctx.Check(download.Close)

		r, err := gzip.NewReader(download)
		if err != nil {
			t.Fatalf("NewReader: %v", err)
		}
		defer ctx.Check(r.Close)

		b, err := io.ReadAll(r)
		if err != nil {
			t.Fatalf("ReadAll: %v", err)
		}

		if got, want := string(b), testdata; got != want {
			t.Errorf("Downloaded data isn't equal to uploaded data: got %v, want %v", got, want)
		}
	})
}
