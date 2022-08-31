# The SANFT Protocol Go Implementation

## Compiling and Installing Instructions
TODO

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

## Assignment Task: Briefly record what you did and what you learned
### How is your program structured?
The implementation is divided into 5 major Go packages: firstly, we have the general implementation of messages
including the sending an receiving of them in the `messages` package. Next, the implementations 
of server and client can be found in the respective `server` and `client` packages.
Finally, the implementation of the packet loss simulation can be found in the `markov` package.
In addition to these 4 rather specific packages, the general `main` package ties all components together and
provides the implementation of the CLI.

### Which were the major implementation issues?
To our surprise, most of the implementation went rather smoothly. However, inevitably, there were some minor
inconveniences, albeit none which proved to be significant hurdles or impediments.

Firstly, when writing a protocol specification, 48 bit sized fields might seem like a good (or at least innocuous) idea;
however, when implementing a protocol, 48 bit sized fields quickly become an encumbrance, since there is no data type
thorough which they could be represented elegantly.

A significant challenge which is sightly more configuration rather than implementation related was the task of
coming up with useful default values. In our specification, we deliberately choose to let the server decide on certain
protocol specific parameters, most notably the chunk size, the number of packets per second by which the packet rate
is linearly increased, and the maximum number of chunks allowed in an ACR. When testing our implementation, we quickly
observed that tuning these values often has a significant impact on the performance of the file transfer. However, it is
by no means obvious which configuration would be a good default suitable for all scenarios. For example, since our
congestion control algorithm does not include a multiplicative increase phase akin to TCP slow start which can quickly
bump up the packet rate on a new transfer, it would often be desirable to use a high value for the rate increase
parameter.  However, if the file being transferred is rather large, this might slow down the transfer in the long run
as it is more likely to cause congestion.

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

### What would you do differently if you started all over again?
* No 48 bit field sizes!
* Make sure the spec covers all edge cases.

