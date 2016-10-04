package tar_utils

import (
	"archive/tar"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

func Untar(tarStream io.Reader, targetDir string) error {
	tarBallReader := tar.NewReader(tarStream)

	// Extracting tarred files

	for {
		header, err := tarBallReader.Next()
		if err != nil {
			if err == io.EOF {
				break
			}

			return err
		}

		// get the individual filename and extract to the target directory
		filename := header.Name

		switch header.Typeflag {
		case tar.TypeDir:
			// handle directory
			err = MkdirAll(targetDir, filename, os.FileMode(header.Mode)) // or use 0755 if you prefer

			if err != nil {
				return err
			}

		case tar.TypeReg:
			fullFilename := filepath.Join(targetDir, filename)
			dir := filepath.Dir(filename)

			err = MkdirAll(targetDir, dir, os.FileMode(header.Mode)) // or use 0755 if you prefer

			if err != nil {
				return err
			}

			// handle normal file
			writer, err := os.Create(fullFilename)

			if err != nil {
				return err
			}

			io.Copy(writer, tarBallReader)

			err = os.Chmod(fullFilename, os.FileMode(header.Mode))

			if err != nil {
				return err
			}

			writer.Close()
		}
	}

	return nil
}

// Source: http://blog.ralch.com/tutorial/golang-working-with-tar-and-gzip/
func Tarit(source string, target io.WriteCloser) error {
	defer target.Close()

	tarball := tar.NewWriter(target)
	defer tarball.Close()

	info, err := os.Stat(source)
	if err != nil {
		return nil
	}

	var baseDir string
	if info.IsDir() {
		baseDir = filepath.Base(source)
	}

	return filepath.Walk(source,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			header, err := tar.FileInfoHeader(info, info.Name())
			if err != nil {
				return err
			}

			if baseDir != "" {
				header.Name = filepath.Join(baseDir, strings.TrimPrefix(path, source))
			}

			if err := tarball.WriteHeader(header); err != nil {
				return err
			}

			if info.IsDir() {
				return nil
			}

			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()
			_, err = io.Copy(tarball, file)
			return err
		})
}

func MkdirAll(basePath, targetDir string, fileMode os.FileMode) error {
	parents := []string{filepath.Clean(targetDir)}
	prevParent := ""

	for {
		parent := filepath.Dir(targetDir)
		if parent == prevParent {
			break
		}
		prevParent = parent

		parents = append(parents, parent)
		targetDir = parent
	}

	for i := len(parents) - 1; i >= 0; i-- {
		fullDirPath := filepath.Join(basePath, parents[i])

		dir, err := os.Stat(fullDirPath)
		if err == nil {
			if dir.IsDir() {
				continue
			}
			return &os.PathError{"mkdir", fullDirPath, syscall.ENOTDIR}
		}

		err = os.Mkdir(fullDirPath, fileMode)
		if err != nil {
			// Handle arguments like "foo/." by
			// double-checking that directory doesn't exist.
			dir, err1 := os.Lstat(fullDirPath)
			if err1 != nil || !dir.IsDir() {
				return err
			}
		}
	}

	return nil
}
