package storage

import (
	"errors"
	"testing"
	"time"

	"code.uber.internal/infra/memtsdb"
	"code.uber.internal/infra/memtsdb/mocks"
	"code.uber.internal/infra/memtsdb/sharding"

	"github.com/golang/mock/gomock"
	"github.com/spaolacci/murmur3"
	"github.com/stretchr/testify/require"
)

func testShardingScheme(t *testing.T) sharding.ShardScheme {
	shardScheme, err := sharding.NewShardScheme(0, 1, func(id string) uint32 {
		return murmur3.Sum32([]byte(id)) % 1024
	})
	require.NoError(t, err)
	return shardScheme
}

func testDatabaseOptions() memtsdb.DatabaseOptions {
	var opts memtsdb.DatabaseOptions
	opts = NewDatabaseOptions().
		NowFn(time.Now).
		BufferFuture(10 * time.Minute).
		BufferPast(10 * time.Minute).
		BufferDrain(10 * time.Minute).
		BlockSize(2 * time.Hour).
		RetentionPeriod(2 * 24 * time.Hour).
		MaxFlushRetries(3)
	return opts
}

func testDatabase(t *testing.T) *db {
	ss := testShardingScheme(t)
	opts := testDatabaseOptions()
	return NewDatabase(ss.All(), opts).(*db)
}

func TestNeedDiskFlushDuringBootstrap(t *testing.T) {
	database := testDatabase(t)
	now := database.opts.GetNowFn()()
	require.False(t, database.needDiskFlush(now))
	database.bs = bootstrapped
	require.True(t, database.needDiskFlush(now))
}

func TestNeedDiskFlushWhileFlushing(t *testing.T) {
	database := testDatabase(t)
	database.bs = bootstrapped
	now := database.opts.GetNowFn()()
	require.True(t, database.needDiskFlush(now))
	database.fs = flushInProgress
	require.False(t, database.needDiskFlush(now))
}

func TestNeedDiskFlushAttemptedBefore(t *testing.T) {
	database := testDatabase(t)
	database.bs = bootstrapped
	now := database.opts.GetNowFn()()
	require.True(t, database.needDiskFlush(now))
	firstBlockStart := now.Add(-2 * time.Hour).Add(-10 * time.Minute).Truncate(2 * time.Hour)
	database.flushAttempted[firstBlockStart] = flushState{status: flushInProgress}
	require.False(t, database.needDiskFlush(now))
}

func TestGetFirstBlockStart(t *testing.T) {
	inputs := []struct {
		tickStart time.Time
		expected  time.Time
	}{
		{time.Unix(14900, 0), time.Unix(0, 0)},
		{time.Unix(15000, 0), time.Unix(7200, 0)},
		{time.Unix(15100, 0), time.Unix(7200, 0)},
	}
	database := testDatabase(t)
	for _, input := range inputs {
		require.Equal(t, input.expected, database.getFirstBlockStart(input.tickStart))
	}
}

func TestFlushToDisk(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	inputTimes := []struct {
		bs time.Time
		fs flushState
	}{
		{time.Unix(21600, 0), flushState{flushFailed, 2}},
		{time.Unix(28800, 0), flushState{flushFailed, 3}},
		{time.Unix(36000, 0), flushState{flushInProgress, 0}},
		{time.Unix(43200, 0), flushState{flushSuccess, 1}},
	}
	notFlushed := make(map[time.Time]struct{})
	for i := 1; i < 4; i++ {
		notFlushed[inputTimes[i].bs] = struct{}{}
	}

	tickStart := time.Unix(188000, 0)
	database := testDatabase(t)
	for _, input := range inputTimes {
		database.flushAttempted[input.bs] = input.fs
	}
	endTime := time.Unix(0, 0).Add(2 * 24 * time.Hour)
	for shard := 0; shard < 2; shard++ {
		m := mocks.NewMockdatabaseShard(ctrl)
		database.shards[shard] = m
		m.EXPECT().ShardNum().Return(uint32(shard))
		cur := inputTimes[0].bs
		for !cur.After(endTime) {
			if _, excluded := notFlushed[cur]; !excluded {
				m.EXPECT().FlushToDisk(cur).Return(nil)
			}
			cur = cur.Add(2 * time.Hour)
		}
		m.EXPECT().FlushToDisk(cur).Return(errors.New("some errors"))
	}

	database.flushToDisk(tickStart, false)

	j := 0
	for i := 0; i < 19; i++ {
		if i == 1 {
			j += 3
		}
		expectedTime := time.Unix(int64(21600+j*7200), 0)
		require.Equal(t, flushSuccess, database.flushAttempted[expectedTime].status)
		j++
	}
	expectedTime := time.Unix(int64(180000), 0)
	require.Equal(t, flushFailed, database.flushAttempted[expectedTime].status)
	require.Equal(t, 1, database.flushAttempted[expectedTime].numFailures)
	require.Equal(t, flushNotStarted, database.fs)
}

func TestGetTimesToFlush(t *testing.T) {
	inputTimes := []struct {
		bs time.Time
		fs flushState
	}{
		{time.Unix(21600, 0), flushState{flushFailed, 2}},
		{time.Unix(28800, 0), flushState{flushFailed, 3}},
		{time.Unix(36000, 0), flushState{flushInProgress, 0}},
		{time.Unix(43200, 0), flushState{flushSuccess, 1}},
	}
	database := testDatabase(t)
	for _, input := range inputTimes {
		database.flushAttempted[input.bs] = input.fs
	}
	tickStart := time.Unix(188000, 0)
	res := database.getTimesToFlush(tickStart)
	require.Equal(t, 20, len(res))
	j := 0
	for i := 0; i < 20; i++ {
		if i == 1 {
			j += 3
		}
		expectedTime := time.Unix(int64(21600+j*7200), 0)
		require.Equal(t, expectedTime, res[19-i])
		require.Equal(t, flushInProgress, database.flushAttempted[expectedTime].status)
		j++
	}
}

func TestFlushToDiskWithTimes(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	database := testDatabase(t)
	flushTime := time.Unix(7200, 0)

	for i := 0; i < 2; i++ {
		m := mocks.NewMockdatabaseShard(ctrl)
		database.shards[i] = m
		m.EXPECT().FlushToDisk(flushTime).Return(nil)
	}
	require.True(t, database.flushToDiskWithTime(flushTime))

	m := mocks.NewMockdatabaseShard(ctrl)
	database.shards[0] = m
	m.EXPECT().FlushToDisk(flushTime).Return(nil)

	m = mocks.NewMockdatabaseShard(ctrl)
	database.shards[1] = m
	m.EXPECT().FlushToDisk(flushTime).Return(errors.New("some errors"))
	m.EXPECT().ShardNum().Return(uint32(1))

	require.False(t, database.flushToDiskWithTime(flushTime))
}