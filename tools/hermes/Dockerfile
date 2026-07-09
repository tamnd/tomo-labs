# The hermes tool image: the shared base plus the Hermes Agent and the adapter
# that drives it. hermes-agent on npm is a thin bridge whose postinstall step
# pip installs the upstream Python Hermes Agent (by Nous Research), so the real
# agent is a Python runtime that the bridge launches. The base already carries
# Node 22 and Python 3.11, which the bridge and the agent both need.
FROM tomolab-base
# Debian bookworm marks its system Python as externally managed (PEP 668), so the
# bridge's `pip install hermes-agent` needs this to be allowed at image build.
ENV PIP_BREAK_SYSTEM_PACKAGES=1
ARG HERMES_VERSION=latest
RUN npm install -g hermes-agent@${HERMES_VERSION}
COPY adapter.sh /usr/local/bin/adapter
RUN chmod +x /usr/local/bin/adapter
ENTRYPOINT ["/usr/local/bin/adapter"]
