vcl 4.1;

backend default {
    .host = "fileserver";
    .port = "80";
}

sub vcl_backend_response {
    if (beresp.ttl <= 0s) {
        set beresp.ttl = 5m;
    }
}
