A minimal file server for unix systems which serves files as if they exist on
a case-insensitive file system.

# Usage

Serve files from the current directory

```
go get -u github.com/tiehuis/cihttp
cihttp
```

# Why

If an application is developed on Windows and the developer has not normalized
their file paths then it is common for many 404 errors to occur as unlike windows,
unix systems have case-sensitive file systems.

This server simply remaps requests to look for files in a case insensitive manner
regardless of the underlying file-system case sensitivity.
