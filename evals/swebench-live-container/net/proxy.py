import aiohttp, json, os, time
from aiohttp import web

UPSTREAM  = os.environ.get("UPSTREAM", "https://opencode.ai")
USAGE_LOG = os.environ.get("USAGE_LOG", "/trace/usage.jsonl")

def norm_usage(u):
    # Normalise chat and responses usage into one schema so downstream
    # aggregation reads the same fields regardless of wire dialect.
    if not u:
        return None
    if "prompt_tokens" in u or "completion_tokens" in u:
        return u  # already chat shape
    # Responses shape -> map onto the chat field names we log elsewhere.
    itd = u.get("input_tokens_details") or {}
    otd = u.get("output_tokens_details") or {}
    cached = itd.get("cached_tokens", 0)
    pt = u.get("input_tokens", 0)
    return {
        "prompt_tokens": pt,
        "completion_tokens": u.get("output_tokens", 0),
        "total_tokens": u.get("total_tokens", 0),
        "prompt_cache_hit_tokens": cached,
        "prompt_cache_miss_tokens": max(pt - cached, 0),
        "prompt_tokens_details": {"cached_tokens": cached},
        "completion_tokens_details": {"reasoning_tokens": otd.get("reasoning_tokens", 0)},
        "cache_write_tokens": itd.get("cache_write_tokens", 0),
    }

async def handle(request):
    body = await request.read()
    model = None
    is_chat = False
    try:
        j = json.loads(body)
        model = j.get("model")
        is_chat = "messages" in j
        if j.get("stream") and is_chat:            # force usage on streamed chat calls
            j.setdefault("stream_options", {})["include_usage"] = True
            body = json.dumps(j).encode()
    except Exception:
        pass
    url  = UPSTREAM + request.rel_url.path_qs
    hdrs = {k: v for k, v in request.headers.items()
            if k.lower() not in ("host", "content-length", "accept-encoding")}
    hdrs["Accept-Encoding"] = "identity"          # no compression, easy to parse
    session = aiohttp.ClientSession(timeout=aiohttp.ClientTimeout(total=None, sock_read=600))
    up = await session.request(request.method, url, data=body, headers=hdrs)
    out_hdrs = {k: v for k, v in up.headers.items()
                if k.lower() not in ("content-length", "transfer-encoding")}
    resp = web.StreamResponse(status=up.status, headers=out_hdrs)
    await resp.prepare(request)
    buf = bytearray()
    async for chunk in up.content.iter_any():
        await resp.write(chunk)
        buf += chunk
    await up.release(); await session.close()
    # extract usage (SSE final chunk or plain JSON), chat or responses dialect
    usage = None
    text = bytes(buf).decode("utf-8", "replace")
    ctype = up.headers.get("Content-Type", "")
    try:
        if "text/event-stream" in ctype or text.lstrip().startswith("data:") or "\ndata:" in text:
            for line in text.splitlines():
                line = line.strip()
                if line.startswith("data:"):
                    d = line[5:].strip()
                    if d and d != "[DONE]":
                        try:
                            o = json.loads(d)
                        except Exception:
                            continue
                        if o.get("usage"):                       # chat streamed usage
                            usage = o["usage"]
                        elif o.get("type") == "response.completed":  # responses usage
                            ru = (o.get("response") or {}).get("usage")
                            if ru:
                                usage = ru
        else:
            o = json.loads(text)
            usage = o.get("usage") or (o.get("response") or {}).get("usage")
    except Exception:
        pass
    rec = {"ts": time.time(), "ua": request.headers.get("User-Agent", ""),
           "status": up.status, "model": model, "usage": norm_usage(usage)}
    try:
        with open(USAGE_LOG, "a") as f: f.write(json.dumps(rec) + "\n")
    except Exception: pass
    return resp

app = web.Application(client_max_size=1024**3)
app.router.add_route("*", "/{tail:.*}", handle)
web.run_app(app, port=8080, access_log=None)
