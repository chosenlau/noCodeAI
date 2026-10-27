package taskstore

import (
	"context"
	"github.com/chosenlau/noCodeAI/internal/core/execution"
	"github.com/chosenlau/noCodeAI/internal/dal/model"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestMySQLVersionHistoryAndRestorationFences(t *testing.T) {
	runs := mysqlStore(t)
	ctx := context.Background()
	s := NewExecutionStore(runs.db)
	// Only the columns used by restoration; the isolated DB has no application data.
	require.NoError(t, runs.db.Exec("CREATE TABLE app (id BIGINT PRIMARY KEY, codeGenType VARCHAR(64), projectArchitecture TEXT)").Error)
	require.NoError(t, runs.db.Exec("INSERT INTO app VALUES (401, 'html', 'old description')").Error)
	c, err := s.Claim(ctx, 401, "first", time.Minute)
	require.NoError(t, err)
	_, err = s.Publish(ctx, execution.Publication{Claim: c, VersionID: 41, Path: "snapshot-one"}, map[string]string{"index.html": "one"}, "html")
	require.NoError(t, err)
	require.NoError(t, s.Release(ctx, c))
	c, err = s.Claim(ctx, 401, "second", time.Minute)
	require.NoError(t, err)
	require.NoError(t, s.RegisterSnapshot(ctx, c, "snapshot-one", "hash-one"))
	bad := c
	bad.Epoch++
	require.ErrorIs(t, s.RegisterSnapshot(ctx, bad, "wrong", "wrong"), execution.ErrFenced)
	row, err := s.SourceVersion(ctx, 401, 41)
	require.NoError(t, err)
	require.Equal(t, "hash-one", row.ContentHash)
	_, err = s.SourceVersion(ctx, 402, 41)
	require.Error(t, err)
	_, err = s.Publish(ctx, execution.Publication{Claim: c, VersionID: 42, Path: "snapshot-two", ContentHash: "hash-two"}, nil, "html")
	require.NoError(t, err)
	require.NoError(t, s.Release(ctx, c))
	c, err = s.Claim(ctx, 401, "restore", time.Minute)
	require.NoError(t, err)
	p := execution.Publication{Claim: c, VersionID: 43, Path: "restored-copy", ContentHash: "hash-one", RestoredFromID: 41}
	run, _, err := runs.CreateRun(ctx, request(401, "restore-active"))
	require.NoError(t, err)
	_, err = s.PublishRestoration(ctx, p, nil, "html")
	require.ErrorIs(t, err, execution.ErrBusy)
	_, err = runs.Fail(ctx, run.ID, "stop", false)
	require.NoError(t, err)
	stale := p
	stale.Claim.BaseVersionID = 41
	_, err = s.PublishRestoration(ctx, stale, nil, "html")
	require.ErrorIs(t, err, execution.ErrFenced)
	_, err = s.PublishRestoration(ctx, p, nil, "html")
	require.NoError(t, err)
	current, err := s.Current(ctx, 401)
	require.NoError(t, err)
	require.EqualValues(t, 43, current.ID)
	row, err = s.SourceVersion(ctx, 401, 43)
	require.NoError(t, err)
	require.EqualValues(t, 41, row.RestoredFromID)
	require.Equal(t, "hash-one", row.ContentHash)
	rows, err := s.Versions(ctx, 401)
	require.NoError(t, err)
	require.Len(t, rows, 3)
	rows, err = s.Versions(ctx, 402)
	require.NoError(t, err)
	require.Empty(t, rows)
	var app model.App
	require.NoError(t, runs.db.First(&app, 401).Error)
	require.Empty(t, app.ProjectArchitecture)
	_, err = s.PublishRestoration(ctx, p, nil, "html")
	require.ErrorIs(t, err, execution.ErrFenced)
}
