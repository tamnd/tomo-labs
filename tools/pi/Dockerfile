# The pi tool image: the shared base plus the pi coding agent and the adapter
# that drives it. pi is installed from npm (package
# @earendil-works/pi-coding-agent, binary pi) with --ignore-scripts, the install
# pi's own docs prescribe, so this image is independent of any pi checkout on the
# host. The base already carries Node 22, which pi needs.
FROM tomolab-base
ARG PI_VERSION=0.80.7
RUN npm install -g --ignore-scripts @earendil-works/pi-coding-agent@${PI_VERSION}
COPY adapter.sh /usr/local/bin/adapter
RUN chmod +x /usr/local/bin/adapter
ENTRYPOINT ["/usr/local/bin/adapter"]
