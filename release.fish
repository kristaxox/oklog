#!/usr/bin/env fish

function version_prompt
	echo "Semantic version: "
end

set VERSION (cat ./version)

set REV (git rev-parse --short HEAD)
echo Tagging $REV as v$VERSION
git tag --annotate v$VERSION -m "Release v$VERSION"
echo Be sure to: git push --tags
echo

set DISTDIR dist/v$VERSION
mkdir -p $DISTDIR

for pair in linux/386 linux/amd64 linux/arm linux/arm64 darwin/amd64 darwin/arm64 dragonfly/amd64 freebsd/amd64 netbsd/amd64 openbsd/amd64
	set GOOS   (echo $pair | cut -d'/' -f1)
	set GOARCH (echo $pair | cut -d'/' -f2)
	set BIN    $DISTDIR/oklog-$VERSION-$GOOS-$GOARCH
	echo $BIN
	env GOOS=$GOOS GOARCH=$GOARCH go build -o $BIN -ldflags="-X main.version=$VERSION" ./cmd/oklog
end

