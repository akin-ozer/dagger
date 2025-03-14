package buildkit

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/containerd/continuity/fs"
	"github.com/dagger/dagger/engine"
	"github.com/dagger/dagger/engine/sources/blob"
	cacheconfig "github.com/moby/buildkit/cache/config"
	bkclient "github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"
	bkgw "github.com/moby/buildkit/frontend/gateway/client"
	"github.com/moby/buildkit/session/filesync"
	"github.com/moby/buildkit/snapshot"
	bksolverpb "github.com/moby/buildkit/solver/pb"
	"github.com/moby/buildkit/util/compression"
	bkworker "github.com/moby/buildkit/worker"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/vito/progrock"
)

func (c *Client) LocalImport(
	ctx context.Context,
	recorder *progrock.Recorder,
	platform specs.Platform,
	srcPath string,
	excludePatterns []string,
	includePatterns []string,
) (*bksolverpb.Definition, error) {
	srcPath = path.Clean(srcPath)
	if srcPath == ".." || strings.HasPrefix(srcPath, "../") {
		return nil, fmt.Errorf("path %q escapes workdir; use an absolute path instead", srcPath)
	}

	clientMetadata, err := engine.ClientMetadataFromContext(ctx)
	if err != nil {
		return nil, err
	}

	localOpts := []llb.LocalOption{
		llb.SessionID(clientMetadata.ClientID),
		llb.SharedKeyHint(strings.Join([]string{clientMetadata.ClientHostname, srcPath}, " ")),
	}

	localName := fmt.Sprintf("upload %s from %s (client id: %s)", srcPath, clientMetadata.ClientHostname, clientMetadata.ClientID)
	if len(excludePatterns) > 0 {
		localName += fmt.Sprintf(" (exclude: %s)", strings.Join(excludePatterns, ", "))
		localOpts = append(localOpts, llb.ExcludePatterns(excludePatterns))
	}
	if len(includePatterns) > 0 {
		localName += fmt.Sprintf(" (include: %s)", strings.Join(includePatterns, ", "))
		localOpts = append(localOpts, llb.IncludePatterns(includePatterns))
	}
	localOpts = append(localOpts, llb.WithCustomName(localName))
	localLLB := llb.Local(srcPath, localOpts...)

	// We still need to do a copy here for now because buildkit's cache calls Finalize on refs when getting their blobs
	// which makes the cache ref for the local ref unable to be reused.
	copyLLB := llb.Scratch().File(
		llb.Copy(localLLB, "/", "/"),
		llb.WithCustomNamef(localName),
	)

	copyDef, err := copyLLB.Marshal(ctx, llb.Platform(platform))
	if err != nil {
		return nil, err
	}
	copyPB := copyDef.ToPB()

	RecordVertexes(recorder, copyPB)

	res, err := c.Solve(ctx, bkgw.SolveRequest{
		Definition: copyPB,
		Evaluate:   true,
	})
	if err != nil {
		return nil, err
	}
	resultProxy, err := res.SingleRef()
	if err != nil {
		return nil, fmt.Errorf("failed to get single ref: %s", err)
	}
	cachedRes, err := resultProxy.Result(ctx)
	if err != nil {
		return nil, wrapError(ctx, err, c.ID())
	}
	workerRef, ok := cachedRes.Sys().(*bkworker.WorkerRef)
	if !ok {
		return nil, fmt.Errorf("invalid ref: %T", cachedRes.Sys())
	}
	ref := workerRef.ImmutableRef

	// Force an unlazy of the copy in case it was lazy due to remote caching; we
	// need it to exist locally or else blob source won't work.
	// NOTE: in theory we could keep it lazy if we could get the descriptor handlers
	// for the remote over to the blob source code, but the plumbing to accomplish that
	// is tricky and ultimately only result in a marginal performance optimization.
	err = ref.Extract(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to extract ref: %s", err)
	}

	remotes, err := ref.GetRemotes(ctx, true, cacheconfig.RefConfig{
		Compression: compression.Config{
			// TODO: double check whether using Zstd is best idea. It's the fastest, but
			// if it ends up in an exported image and the user tries to load that into
			// old docker versions, they will get an error unless they specify the force
			// compression option during export.
			Type: compression.Zstd,
		},
	}, false, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get remotes: %s", err)
	}
	if len(remotes) != 1 {
		return nil, fmt.Errorf("expected 1 remote, got %d", len(remotes))
	}
	remote := remotes[0]
	if len(remote.Descriptors) != 1 {
		return nil, fmt.Errorf("expected 1 descriptor, got %d", len(remote.Descriptors))
	}
	desc := remote.Descriptors[0]

	blobDef, err := blob.LLB(desc).Marshal(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal blob source: %s", err)
	}
	blobPB := blobDef.ToPB()

	// do a sync solve right now so we can release the cache ref for the local import
	// without giving up the lease on the blob
	_, err = c.Solve(ctx, bkgw.SolveRequest{
		Definition: blobPB,
		Evaluate:   true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to solve blobsource: %w", wrapError(ctx, err, c.ID()))
	}

	return blobPB, nil
}

// Import a directory from the engine container, as opposed to from a client
func (c *Client) EngineContainerLocalImport(
	ctx context.Context,
	recorder *progrock.Recorder,
	platform specs.Platform,
	srcPath string,
	excludePatterns []string,
	includePatterns []string,
) (*bksolverpb.Definition, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("failed to get hostname for engine local import: %s", err)
	}
	ctx = engine.ContextWithClientMetadata(ctx, &engine.ClientMetadata{
		ClientID:       c.ID(),
		ClientHostname: hostname,
	})

	return c.LocalImport(ctx, recorder, platform, srcPath, excludePatterns, includePatterns)
}

func (c *Client) ReadCallerHostFile(ctx context.Context, path string) ([]byte, error) {
	ctx, cancel, err := c.withClientCloseCancel(ctx)
	if err != nil {
		return nil, err
	}
	defer cancel()

	clientMetadata, err := engine.ClientMetadataFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get requester session ID: %s", err)
	}

	ctx = engine.LocalImportOpts{
		OwnerClientID:      clientMetadata.ClientID,
		Path:               path,
		ReadSingleFileOnly: true,
		MaxFileSize:        MaxFileContentsChunkSize,
	}.AppendToOutgoingContext(ctx)

	clientCaller, err := c.SessionManager.Get(ctx, clientMetadata.ClientID, false)
	if err != nil {
		return nil, fmt.Errorf("failed to get requester session: %s", err)
	}
	diffCopyClient, err := filesync.NewFileSyncClient(clientCaller.Conn()).DiffCopy(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create diff copy client: %s", err)
	}
	defer diffCopyClient.CloseSend()
	msg := filesync.BytesMessage{}
	err = diffCopyClient.RecvMsg(&msg)
	if err != nil {
		return nil, fmt.Errorf("failed to receive file bytes message: %s", err)
	}
	return msg.Data, nil
}

func (c *Client) LocalDirExport(
	ctx context.Context,
	def *bksolverpb.Definition,
	destPath string,
) error {
	ctx, cancel, err := c.withClientCloseCancel(ctx)
	if err != nil {
		return err
	}
	defer cancel()

	destPath = path.Clean(destPath)
	if destPath == ".." || strings.HasPrefix(destPath, "../") {
		return fmt.Errorf("path %q escapes workdir; use an absolute path instead", destPath)
	}

	res, err := c.Solve(ctx, bkgw.SolveRequest{Definition: def})
	if err != nil {
		return fmt.Errorf("failed to solve for local export: %s", err)
	}
	cacheRes, err := ConvertToWorkerCacheResult(ctx, res)
	if err != nil {
		return fmt.Errorf("failed to convert result: %s", err)
	}

	exporter, err := c.Worker.Exporter(bkclient.ExporterLocal, c.SessionManager)
	if err != nil {
		return err
	}

	expInstance, err := exporter.Resolve(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to resolve exporter: %s", err)
	}

	clientMetadata, err := engine.ClientMetadataFromContext(ctx)
	if err != nil {
		return fmt.Errorf("failed to get requester session ID: %s", err)
	}

	ctx = engine.LocalExportOpts{
		DestClientID: clientMetadata.ClientID,
		Path:         destPath,
	}.AppendToOutgoingContext(ctx)

	_, descRef, err := expInstance.Export(ctx, cacheRes, clientMetadata.ClientID)
	if err != nil {
		return fmt.Errorf("failed to export: %s", err)
	}
	if descRef != nil {
		descRef.Release()
	}
	return nil
}

func (c *Client) LocalFileExport(
	ctx context.Context,
	def *bksolverpb.Definition,
	destPath string,
	filePath string,
	allowParentDirPath bool,
) error {
	ctx, cancel, err := c.withClientCloseCancel(ctx)
	if err != nil {
		return err
	}
	defer cancel()

	destPath = path.Clean(destPath)
	if destPath == ".." || strings.HasPrefix(destPath, "../") {
		return fmt.Errorf("path %q escapes workdir; use an absolute path instead", destPath)
	}

	res, err := c.Solve(ctx, bkgw.SolveRequest{Definition: def, Evaluate: true})
	if err != nil {
		return fmt.Errorf("failed to solve for local export: %s", err)
	}
	ref, err := res.SingleRef()
	if err != nil {
		return fmt.Errorf("failed to get single ref: %s", err)
	}

	mountable, err := ref.getMountable(ctx)
	if err != nil {
		return fmt.Errorf("failed to get mountable: %s", err)
	}
	mounter := snapshot.LocalMounter(mountable)
	mountPath, err := mounter.Mount()
	if err != nil {
		return fmt.Errorf("failed to mount: %s", err)
	}
	defer mounter.Unmount()
	mntFilePath, err := fs.RootPath(mountPath, filePath)
	if err != nil {
		return fmt.Errorf("failed to get root path: %s", err)
	}
	file, err := os.Open(mntFilePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %s", err)
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat file: %s", err)
	}

	clientMetadata, err := engine.ClientMetadataFromContext(ctx)
	if err != nil {
		return fmt.Errorf("failed to get requester session ID: %s", err)
	}

	ctx = engine.LocalExportOpts{
		DestClientID:       clientMetadata.ClientID,
		Path:               destPath,
		IsFileStream:       true,
		FileOriginalName:   filepath.Base(filePath),
		AllowParentDirPath: allowParentDirPath,
		FileMode:           stat.Mode().Perm(),
	}.AppendToOutgoingContext(ctx)

	clientCaller, err := c.SessionManager.Get(ctx, clientMetadata.ClientID, false)
	if err != nil {
		return fmt.Errorf("failed to get requester session: %s", err)
	}
	diffCopyClient, err := filesync.NewFileSendClient(clientCaller.Conn()).DiffCopy(ctx)
	if err != nil {
		return fmt.Errorf("failed to create diff copy client: %s", err)
	}
	defer diffCopyClient.CloseSend()

	fileStat, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat file: %s", err)
	}
	fileSizeLeft := fileStat.Size()
	chunkSize := int64(MaxFileContentsChunkSize)
	for fileSizeLeft > 0 {
		buf := new(bytes.Buffer) // TODO: more efficient to use bufio.Writer, reuse buffers, sync.Pool, etc.
		n, err := io.CopyN(buf, file, chunkSize)
		if errors.Is(err, io.EOF) {
			err = nil
		}
		if err != nil {
			return fmt.Errorf("failed to read file: %s", err)
		}
		fileSizeLeft -= n
		err = diffCopyClient.SendMsg(&filesync.BytesMessage{Data: buf.Bytes()})
		if errors.Is(err, io.EOF) {
			err := diffCopyClient.RecvMsg(struct{}{})
			if err != nil {
				return fmt.Errorf("diff copy client error: %s", err)
			}
		} else if err != nil {
			return fmt.Errorf("failed to send file chunk: %s", err)
		}
	}
	if err := diffCopyClient.CloseSend(); err != nil {
		return fmt.Errorf("failed to close send: %s", err)
	}
	// wait for receiver to finish
	var msg filesync.BytesMessage
	if err := diffCopyClient.RecvMsg(&msg); err != io.EOF {
		return fmt.Errorf("unexpected closing recv msg: %s", err)
	}
	return nil
}

// IOReaderExport exports the contents of an io.Reader to the caller's local fs as a file
// TODO: de-dupe this with the above method to extent possible
func (c *Client) IOReaderExport(ctx context.Context, r io.Reader, destPath string, destMode os.FileMode) error {
	clientMetadata, err := engine.ClientMetadataFromContext(ctx)
	if err != nil {
		return fmt.Errorf("failed to get requester session ID: %s", err)
	}

	ctx = engine.LocalExportOpts{
		DestClientID:     clientMetadata.ClientID,
		Path:             destPath,
		IsFileStream:     true,
		FileOriginalName: filepath.Base(destPath),
		FileMode:         destMode,
	}.AppendToOutgoingContext(ctx)

	clientCaller, err := c.SessionManager.Get(ctx, clientMetadata.ClientID, false)
	if err != nil {
		return fmt.Errorf("failed to get requester session: %s", err)
	}
	diffCopyClient, err := filesync.NewFileSendClient(clientCaller.Conn()).DiffCopy(ctx)
	if err != nil {
		return fmt.Errorf("failed to create diff copy client: %s", err)
	}
	defer diffCopyClient.CloseSend()

	chunkSize := int64(MaxFileContentsChunkSize)
	keepGoing := true
	for keepGoing {
		buf := new(bytes.Buffer) // TODO: more efficient to use bufio.Writer, reuse buffers, sync.Pool, etc.
		_, err := io.CopyN(buf, r, chunkSize)
		if errors.Is(err, io.EOF) {
			keepGoing = false
			err = nil
		}
		if err != nil {
			return fmt.Errorf("failed to read file: %s", err)
		}
		err = diffCopyClient.SendMsg(&filesync.BytesMessage{Data: buf.Bytes()})
		if errors.Is(err, io.EOF) {
			err := diffCopyClient.RecvMsg(struct{}{})
			if err != nil {
				return fmt.Errorf("diff copy client error: %s", err)
			}
		} else if err != nil {
			return fmt.Errorf("failed to send file chunk: %s", err)
		}
	}
	if err := diffCopyClient.CloseSend(); err != nil {
		return fmt.Errorf("failed to close send: %s", err)
	}
	// wait for receiver to finish
	var msg filesync.BytesMessage
	if err := diffCopyClient.RecvMsg(&msg); err != io.EOF {
		return fmt.Errorf("unexpected closing recv msg: %s", err)
	}
	return nil
}
