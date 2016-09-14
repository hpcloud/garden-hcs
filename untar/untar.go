package untar

import (
	"archive/tar"
	"io"
	"os"
	"path/filepath"
	"strings"
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
		fullFilename := "\\\\?\\" + filepath.Join(targetDir, filename)

		switch header.Typeflag {
		case tar.TypeDir:
			// handle directory
			err = os.MkdirAll(fullFilename, os.FileMode(header.Mode)) // or use 0755 if you prefer

			if err != nil {
				return err
			}

		case tar.TypeReg:
			fullDirectory := filepath.Dir(fullFilename)

			err = os.MkdirAll(fullDirectory, os.FileMode(header.Mode)) // or use 0755 if you prefer

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

			file, err := os.Open("\\\\?\\" + path)
			if err != nil {
				return err
			}
			defer file.Close()
			_, err = io.Copy(tarball, file)
			return err
		})
}
