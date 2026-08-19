vcl 4.1;

backend default {
    .host = "mock-origin";
    .port = "8080";
}

backend fileserver {
    .host = "fileserver";
    .port = "80";
}

sub vcl_recv {
    if (req.url ~ "^/file/") {
        set req.backend_hint = fileserver;
        set req.url = regsub(req.url, "^/file/([^?]*).*", "/\1");
        return (hash);
    } else {
        set req.backend_hint = default;
    }
}

sub vcl_backend_response {
    set beresp.ttl = 10s;
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
