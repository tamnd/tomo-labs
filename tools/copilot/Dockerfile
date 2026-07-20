# The copilot tool image: the shared base plus the GitHub Copilot CLI and the
# adapter that drives it. Copilot is installed from npm (package @github/copilot,
# binary copilot), so this image is independent of any copilot checkout on the
# host. The base already carries Node 22, which the CLI needs.
FROM tomolab-base
ARG COPILOT_VERSION=1.0.71
RUN npm install -g @github/copilot@${COPILOT_VERSION}
COPY adapter.sh /usr/local/bin/adapter
RUN chmod +x /usr/local/bin/adapter
ENTRYPOINT ["/usr/local/bin/adapter"]
