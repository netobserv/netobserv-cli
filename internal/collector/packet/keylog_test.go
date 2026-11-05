package packet

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseKeylogLinesRemovesCommentsAndBlankLines(t *testing.T) {
	input := "# comment\n\nCLIENT_RANDOM aaa bbb\n  SERVER_HANDSHAKE_TRAFFIC_SECRET ccc ddd  \n"

	lines, err := ParseKeylogLines(strings.NewReader(input))
	require.NoError(t, err)
	assert.Equal(t, "CLIENT_RANDOM aaa bbb\nSERVER_HANDSHAKE_TRAFFIC_SECRET ccc ddd\n", string(lines))
}

func TestParseKeylogLinesReturnsScannerErrors(t *testing.T) {
	reader := &failingReader{}
	_, err := ParseKeylogLines(reader)
	assert.Error(t, err)
}

type failingReader struct{}

func (*failingReader) Read([]byte) (int, error) { return 0, assert.AnError }
