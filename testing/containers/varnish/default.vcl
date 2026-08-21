vcl 4.1;

backend default {
    .host = "mock-origin";
    .port = "8080";
}

sub vcl_recv {
    if (req.url ~ "^/file/") {
        set req.backend_hint = default;
        set req.url = regsub(req.url, "^/file/", "/");
        return (hash);
    } else {
        set req.backend_hint = default;
    }
}

sub vcl_backend_response {
    set beresp.ttl = 120s;
    set beresp.grace = 0s;
    set beresp.keep = 0s;
}

sub vcl_deliver {
    if (obj.hits > 0) {
        set resp.http.X-Cache = "HIT";
        set resp.http.Cache-Status = "varnish; hit";
    } else {
        set resp.http.X-Cache = "MISS";
        set resp.http.Cache-Status = "varnish; fwd=uri-miss";
    }
}
