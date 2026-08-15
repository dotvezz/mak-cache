vcl 4.1;

backend default {
    .host = "mock-origin";
    .port = "8080";
}

sub vcl_backend_response {
    if (beresp.ttl <= 0s) {
        set beresp.ttl = 5s;
    }
}
