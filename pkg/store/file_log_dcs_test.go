package store

import (
	"io"
	"testing"

	"github.com/oklog/oklog/pkg/fs"
	"storj.io/common/testcontext"
	"storj.io/storj/private/testplanet"
)

func Test_fileLogDCS_Purgeable(t *testing.T) {}

type nopEventReporter struct{}

// ReportEvent implements EventReporter.
func (nopEventReporter) ReportEvent(Event) {}

func Test_fileTrashSegmentDCS_Purge(t *testing.T) {
	testplanet.Run(t, testplanet.Config{
		SatelliteCount:   1,
		StorageNodeCount: 1,
		UplinkCount:      1,
	}, func(t *testing.T, ctx *testcontext.Context, planet *testplanet.Planet) {
		t.FailNow()

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
			reporter:         nopEventReporter{},
			fileTrashSegment: fileTrashSegment{virtualFS, testfile},
			project:          project,
			bucketName:       bucketName,
		}

		if err = segment.Purge(); err != nil {
			t.Fatalf("Purge: %v", err)
		}

		if !virtualFS.Exists(testfilename) {
			t.Errorf("%s shouldn't exist at this point", testfilename)
		}

		download, err := project.DownloadObject(ctx, bucketName, testfile.Name(), nil)
		if err != nil {
			t.Fatalf("DownloadObject: %v", err)
		}
		defer ctx.Check(download.Close)

		b, err := io.ReadAll(download)
		if err != nil {
			t.Fatalf("ReadAll: %v", err)
		}

		if got, want := string(b), testdata; got != want {
			t.Errorf("Downloaded data isn't equal to uploaded data: got %v, want %v", got, want)
		}
	})
}
