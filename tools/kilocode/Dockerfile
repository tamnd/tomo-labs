# The kilocode tool image: the shared base plus the Kilo Code CLI and the adapter
# that drives it. Kilo Code is an opencode fork, installed from npm (package
# @kilocode/cli, binary kilo). The npm package is a thin launcher; its postinstall
# pulls the prebuilt binary for the image's platform out of an optional dependency,
# so the image is independent of any kilo checkout on the host. The base already
# carries Node 22, which the installer needs.
FROM tomolab-base
ARG KILOCODE_VERSION=7.4.15
RUN npm install -g @kilocode/cli@${KILOCODE_VERSION}
COPY adapter.sh /usr/local/bin/adapter
RUN chmod +x /usr/local/bin/adapter
ENTRYPOINT ["/usr/local/bin/adapter"]
