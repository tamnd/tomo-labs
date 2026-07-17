# The gemini-cli tool image: the shared base plus Google's Gemini CLI and the
# adapter that drives it. gemini-cli is installed from npm (package
# @google/gemini-cli, a Node launcher), so this image is independent of any
# checkout on the host. The base already carries Node 22, which the launcher needs.
FROM tomolab-base
ARG GEMINI_CLI_VERSION=0.51.0
RUN npm install -g @google/gemini-cli@${GEMINI_CLI_VERSION}
COPY adapter.sh /usr/local/bin/adapter
RUN chmod +x /usr/local/bin/adapter
ENTRYPOINT ["/usr/local/bin/adapter"]
