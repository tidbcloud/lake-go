package golake

import (
	"context"
	"database/sql/driver"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBatchFilePrivateAndRemovedOnEncodingError(t *testing.T) {
	temp := t.TempDir()
	t.Setenv("TMPDIR", temp)
	t.Setenv("TEMP", temp)
	t.Setenv("TMP", temp)
	conn := &LakeConn{}
	batch, err := conn.prepareBatch(context.Background(), "INSERT INTO test VALUES")
	require.NoError(t, err)
	defer batch.cleanup()
	if runtime.GOOS != "windows" {
		info, err := batch.batchFile.Stat()
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0600), info.Mode().Perm())
	}

	_, err = conn.ExecBatch(context.Background(), "INSERT INTO test VALUES", [][]driver.Value{
		{"sensitive"},
		{array{v: 42}},
	})
	require.ErrorContains(t, err, "expected array or slice")
	files, err := os.ReadDir(temp)
	require.NoError(t, err)
	require.Len(t, files, 1)
	require.Equal(t, filepath.Base(batch.batchFile.Name()), files[0].Name())
}
