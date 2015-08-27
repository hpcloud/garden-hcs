package untar

import (
	"archive/tar"
	"io"
	"os"
	"path/filepath"
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
		fullFilename := filepath.Join(targetDir, filename)

		switch header.Typeflag {
		case tar.TypeDir:
			// handle directory
			err = os.MkdirAll(fullFilename, os.FileMode(header.Mode)) // or use 0755 if you prefer

			if err != nil {
				return err
			}

		case tar.TypeReg:
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
