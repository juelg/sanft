# The SANFT Protocol Go Implementation

## Compiling Instructions
First you need to install `Go` on your system. Checkout the [Go documentation](https://go.dev/doc/install) if you are new to go.

In order to compile our project, clone it, cd into it and run `go build`:
```shell
git clone git@gitlab.lrz.de:protocol-design-sose-2022-team-0/sanft.git
cd sanft
go build
```
You will now find a new executable called `sanft`. Checkout its command line options by using
```shell
./sanft --help
```

## Usage
The CLI of the SANFT implementation follows the assignment specification;
to recap:
```
sanft [-s] [-t <port>] [-p <p>] [-q <q>]
sanft <host> [-t <port>] [-p <p>] [-q <q>] <file> ...

-s:	server mode: accept incoming requests from any host
	Operate in client mode if “-s” is not specified
<host> 	the host to request from (hostname or IPv4 address)
-t: 	specify the port number to use (use a default if not given)
-p, -q:	specify the loss probabilities for the Markov chain model
	If only one is specified, assume p=q; if neither is specified assume no
	loss
<file>	the name of the file(s) to fetch
```
Additionally, there are a few more additional flags, some of them specific to the SANFT protocol:

When running in server mode, the server will by default serve all files in the current directory;
an alternative server directory can be specified using the -d flag.

In SANFT, there are several protocol specific values that the server must choose itself depending
on the requirements considering the overall circumstances of the deployment.
The SANFT CLI provides flags that allows the user to specify these values according to his needs.
This includes a `--chunk-size` option to specify the chunk size, the `--rate-increase` option to 
set the number of packets per second that the server which the server adds to the measured rate sent
by the client, and the `--max-chunks-in-acr` flag pertaining to the maximum permitted number of Chunk Requests
in a single ACR, advertised by the server in the Metadata Request Response.

### Examples
Start a simple server on localhost IP 127.0.0.1 with UDP port listening on 9999 and serving from
folder `srv` (relative to current directory)
```shell
./sanft -s 127.0.0.1 -t 9999 -d srv
```
To start a simple client which requests file `test.txt` in `srv` on the server use the following command:
```shell
./sanft 127.0.0.1 -t 9999 test.txt
```

## Tests
Every package except the main one has tests. In order to run theses tests cd into the respective package and run `go test`.

## Assignment Task: Briefly record what you did and what you learned
### How is your program structured?
The implementation is divided into 5 major Go packages: firstly, we have the general implementation of messages
including the sending an receiving of them in the `messages` package. Next, the implementations 
of server and client can be found in the respective `server` and `client` packages.
Finally, the implementation of the packet loss simulation can be found in the `markov` package.
In addition to these 4 rather specific packages, the general `main` package ties all components together and
provides the implementation of the CLI.

### Which were the major implementation issues?
To our surprise, most of the implementation went rather smoothly. This is probably due to our intensive practice of
using test cases where ever possible. Every package except the main package comes with its own test cases.
Thus, most of the bugs could already be addressed during development and when we tested the interoperability of client and server it worked immediately with almost not problems.

However, inevitably, there were some minor inconveniences, albeit none which proved to be significant hurdles or impediments.

Firstly, when writing a protocol specification, 48 bit sized fields might seem like a good (or at least innocuous) idea;
however, when implementing a protocol, 48 bit sized fields quickly become an encumbrance, since there is no data type
thorough which they could be represented elegantly.
Thus, a byte array had to be used to represent the 48 bit field, which required two wrapper functions converting from and to normal
64 bit integers.

A significant challenge which is sightly more configuration rather than implementation related was the task of
coming up with useful default values. In our specification, we deliberately choose to let the server decide on certain
protocol specific parameters, most notably the chunk size, the number of packets per second by which the packet rate
is linearly increased, and the maximum number of chunks allowed in an ACR. When testing our implementation, we quickly
observed that tuning these values often has a significant impact on the performance of the file transfer. However, it is
by no means obvious which configuration would be a good default suitable for all scenarios. For example, since our
congestion control algorithm does not include a multiplicative increase phase akin to TCP slow start which can quickly
bump up the packet rate on a new transfer, it would often be desirable to use a high value for the rate increase
parameter.  However, if the file being transferred is rather large, this might slow down the transfer in the long run
as it is more likely to cause congestion. In the end, we settled on a set of slightly more conservative values that
proved to be beneficial in the circumstances under which we tested our implementation.

Another thing we noticed is that sometimes our specification did not account for certain edge cases, which we will
discuss in the next subsection.

### Did you have to adjust your spec during the implementation?
During the implementation process, we came to the realization that there is a theoretical scenario in which following
the specification to the letter would result in measuring a negative packet rate.
In order for this to happen, the client would have to receive the first CR (with lowest index) after the last CR.
In this case timeExpectedFirst would be larger than timeExpectedLast, thus resulting in a negative packetRate.
Although this situation is unlikely to occur as long as we never have a very high packet rate combined with a
very small ACR, we decided to address this edge case by using another formula when this happens: instead of measuring
a packetRate from the (estimated) times of arrivals, the new packetRate is computed as the previous packetRate multiplied
by the number of received chunks over the number of expected chunks.

On the server side we realized that in the Chunk Request Response for the "Too Many Chunks" error it is not specified if the server has to check first for this error and do not answer any of Chunk Requests or if the server has to process all chunks previous to the chunk which results in the error. We decided to answer all previous Chunk Requests and only answer with the error once it occurs in the answering loop instead of checking for the error beforehand and not answering any requests.
Furthermore, the protocol does not specify any error if the file is larger than the maximal specified size according to the number of chunks. In that case we decided to answer any MDR with a "File Not Found" error as it is impossible to serve the file.

### What would you do differently if you started all over again?
* No 48 bit field sizes!
* Make sure the spec covers all edge cases.

## Copyright
Copyright (C) 2022 Guilhelm Roy, Tobias Jülg, Sebastian Kappes

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.
