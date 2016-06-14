	"time"
	xtime "code.uber.internal/infra/memtsdb/x/time"
	defaultNewFileMode      = os.FileMode(0666)
	defaultNewDirectoryMode = os.ModeDir | os.FileMode(0755)
	blockSize        time.Duration
	filePathPrefix   string
	newFileMode      os.FileMode
	newDirectoryMode os.FileMode
	infoFd             *os.File
	indexFd            *os.File
	dataFd             *os.File
	checkpointFilePath string

	start        time.Time
	infoBuffer   *proto.Buffer
type WriterOptions interface {
	// NewFileMode sets the new file mode.
	NewFileMode(value os.FileMode) WriterOptions

	// GetNewFileMode returns the new file mode.
	GetNewFileMode() os.FileMode

	// NewDirectoryMode sets the new directory mode.
	NewDirectoryMode(value os.FileMode) WriterOptions

	// GetNewDirectoryMode returns the new directory mode.
	GetNewDirectoryMode() os.FileMode
type writerOptions struct {
	newFileMode      os.FileMode
	newDirectoryMode os.FileMode
// NewWriterOptions creates a writer options.
func NewWriterOptions() WriterOptions {
	return &writerOptions{
		newFileMode:      defaultNewFileMode,
		newDirectoryMode: defaultNewDirectoryMode,
}

func (o *writerOptions) NewFileMode(value os.FileMode) WriterOptions {
	opts := *o
	opts.newFileMode = value
	return &opts
}

func (o *writerOptions) GetNewFileMode() os.FileMode {
	return o.newFileMode
}

func (o *writerOptions) NewDirectoryMode(value os.FileMode) WriterOptions {
	opts := *o
	opts.newDirectoryMode = value
	return &opts
}

func (o *writerOptions) GetNewDirectoryMode() os.FileMode {
	return o.newDirectoryMode
}

// NewWriter returns a new writer for a filePathPrefix
func NewWriter(
	blockSize time.Duration,
	filePathPrefix string,
	options WriterOptions,
) Writer {
	if options == nil {
		options = NewWriterOptions()
	return &writer{
		blockSize:        blockSize,
		filePathPrefix:   filePathPrefix,
		newFileMode:      options.GetNewFileMode(),
		newDirectoryMode: options.GetNewDirectoryMode(),
		infoBuffer:       proto.NewBuffer(nil),
		indexBuffer:      proto.NewBuffer(nil),
		varintBuffer:     proto.NewBuffer(nil),
		idxData:          make([]byte, idxLen),
}

// Open initializes the internal state for writing to the given shard,
// specifically creating the shard directory if it doesn't exist, and
// opening / truncating files associated with that shard for writing.
func (w *writer) Open(shard uint32, blockStart time.Time) error {
	shardDir := ShardDirPath(w.filePathPrefix, shard)
	if err := os.MkdirAll(shardDir, w.newDirectoryMode); err != nil {
		return err
	}
	w.start = blockStart
	w.checkpointFilePath = filepathFromTime(shardDir, blockStart, checkpointFileSuffix)
	return openFiles(
		w.openWritable,
		map[string]**os.File{
			filepathFromTime(shardDir, blockStart, infoFileSuffix):  &w.infoFd,
			filepathFromTime(shardDir, blockStart, indexFileSuffix): &w.indexFd,
			filepathFromTime(shardDir, blockStart, dataFileSuffix):  &w.dataFd,
		},
	)
	if len(data) == 0 {
		return nil
	}
	return w.WriteAll(key, [][]byte{data})
}

func (w *writer) WriteAll(key string, data [][]byte) error {
	var size int64
	for _, d := range data {
		size += int64(len(d))
	}
	if size == 0 {
		return nil
	}
	entry.Idx = w.currIdx
	entry.Size = size
	endianness.PutUint64(w.idxData, uint64(w.currIdx))
	for _, d := range data {
		if err := w.writeData(d); err != nil {
			return err
		}
	info := &schema.IndexInfo{
		Start:     xtime.ToNanoseconds(w.start),
		BlockSize: int64(w.blockSize),
		Entries:   w.currIdx,
	}
	if err := w.infoBuffer.Marshal(info); err != nil {
		return err
	}

	if _, err := w.infoFd.Write(w.infoBuffer.Bytes()); err != nil {
	if err := closeFiles(w.infoFd, w.indexFd, w.dataFd); err != nil {
	return w.writeCheckpointFile()
func (w *writer) writeCheckpointFile() error {
	fd, err := w.openWritable(w.checkpointFilePath)
	if err != nil {
		return err
	}
	fd.Close()
	return nil
}

func (w *writer) openWritable(filePath string) (*os.File, error) {
	fd, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, w.newFileMode)
	if err != nil {
		return nil, err
	return fd, nil