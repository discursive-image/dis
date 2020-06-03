# The Discorsive Image Server.
This tool provides a websocket implementation that will serve images with captions from a text source, which could be a file as well as its stdin.

## Installation From Source
(requires [Go](https://golang.org/dl/))
```
% git clone https://github.com/discursive-image/dis.git # clone the repository
% export GO111MODULE=on # (opt) [use go modules](https://blog.golang.org/using-go-modules).
% make # build
```

You'll find the produced executables inside `bin/`. You can start `dis` serving lines from the `examples/hacking-diic_example-1.csv` file by running:
```
 % bin/ratereader < examples/hacking-diic_example-1.csv | bin/dis
```
Or if you want it to read from a file indefinetly (waits for new lines to be written):
```
 % tail -f examples/hacking-diic_example-1.csv | bin/dis
```

You can choose which port `dis` should use using the `-p` flag (checkout `dis --help` for more). When the server starts correctly, a websocket endpoint will be available at `<host>:<port>/di/stream`.

## Specs
Input `csv` file is expected to be formatted as
```
00:05:28.180,00:05:28.330,be,https://lookaside.fbsbx.com/lookaside/crawler/media/?media_id=283494941784
```
The first column being the start duration when the word was first heard (we're talking about speech to text here), second column the end duration, third the spoken word and the last the image link.
