# The opencode tool image: the shared base plus the opencode CLI and the adapter
# that drives it. opencode is installed from npm (package opencode-ai, binary
# opencode), so this image is independent of any opencode checkout on the host.
# The base already carries Node 22, which opencode needs.
FROM tomolab-base
ARG OPENCODE_VERSION=1.18.1
RUN npm install -g opencode-ai@${OPENCODE_VERSION}
COPY adapter.sh /usr/local/bin/adapter
RUN chmod +x /usr/local/bin/adapter
ENTRYPOINT ["/usr/local/bin/adapter"]
