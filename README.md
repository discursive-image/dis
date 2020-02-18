# The Discorsive Image Server.
This tool provides a websocket implementation that will serve images with captions from a text source, which could be a file as well as its stdin.

## Installation From Source
- clone the repository `git clone https://github.com/jecoz/dis.git`
- (opt) use go modules `export GO111MODULE=on`
- build with `make`

You'll find the produced executables inside `bin/`. You can start `dis` serving lines from the `examples/hacking-diic_example-1.csv` file by running:
```
 % bin/ratereader < examples/hacking-diic_example-1.csv | bin/dis
```
Or if you want it to read from a file indefinetly (waits for new lines to be written):
```
 % tail -f examples/hacking-diic_example-1.csv | bin/dis
```

## Specs
Input `csv` file is expected to be formatted as
```
00:05:28.180,00:05:28.330,be,https://lookaside.fbsbx.com/lookaside/crawler/media/?media_id=283494941784
```
The first column being the start duration when the word was first heard (we're talking about speech to text here), second column the end duration, third the spoken word and the last the image link.
