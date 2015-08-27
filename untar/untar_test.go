package untar

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUntar(t *testing.T) {
	assert := assert.New(t)

	workDir, err := os.Getwd()
	assert.Nil(err)

	tarFile := filepath.Join(workDir, "../test-assets/files.tar")
	tarStream, err := os.Open(tarFile)
	assert.Nil(err)

	tempDir, err := ioutil.TempDir("", "garden-untar-tests")
	defer os.RemoveAll(tempDir)

	assert.Nil(err)

	err = Untar(tarStream, tempDir)
	assert.Nil(err)

	filename := filepath.Join(tempDir, "file1.txt")
	_, err = os.Stat(filename)
	assert.Nil(err)

	filename = filepath.Join(tempDir, "dir/file3.txt")
	_, err = os.Stat(filename)
	assert.Nil(err)
}
