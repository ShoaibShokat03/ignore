# Performance Notes

Ignore is designed for large developer workspaces such as React, Next.js, Laravel, Yii2, Node.js, Flutter, and Unity projects.

## Optimizations Implemented

- `filepath.WalkDir` avoids unnecessary `FileInfo` calls during traversal.
- Ignored directories are skipped before traversal enters them.
- File copies run through a bounded worker pool.
- File content is streamed and never loaded wholly into memory.
- Copy buffers are reused through `sync.Pool`.
- Rule files are parsed and cached.
- Project rule cache entries invalidate when project `.ignore` files change.
- Filesystem watcher events are debounced.
- Metrics are updated through small locked sections.
- Logs are rotated at 5 MB with old log pruning.

## Expected Behavior

For a project with 100,000+ files, the largest savings come from skipping high-cardinality directories early:

- `node_modules`
- `.git`
- `vendor`
- `.next`
- `build`
- `dist`
- Unity `Library`
- Flutter `.dart_tool`

The copy engine uses bounded concurrency to avoid saturating memory or overwhelming slower disks. On fast SSDs, increasing `workerCount` in the config may improve throughput. On HDDs or network shares, the default auto setting is usually safer.

## Memory Profile

Memory use is dominated by:

- Worker buffers: `workers * bufferSize`
- Small path queues
- Rule caches
- UI runtime

By default the backend buffer size is 1 MB per worker and workers are capped to a small CPU-based value. Large files are streamed.

