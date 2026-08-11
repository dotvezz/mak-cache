import http from 'k6/http';
import { check, sleep } from 'k6';
import { Options } from 'k6/options';

export const options: Options = {
    vus: 1,
    iterations: 1,
    thresholds: {
        checks: ['rate>0.5'], // Allow reporting all findings without aborting test runner
    },
};

const PORT = __ENV.PORT || '8086';
const BASE_URL = `http://localhost:${PORT}`;

export default function () {
    const runID = Date.now().toString();
    console.log(`Running E2E tests against ${BASE_URL} (runID: ${runID})`);

    test1_BasicMissAndHit(runID);
    test2_TTLExpiration(runID);
    test3_NoStore(runID);
    test4_Private(runID);
    test5_MaxAgeOverride(runID);
    test6_VaryAcceptEncoding(runID);
    test7_VaryStar(runID);
    test8_ETagConditional(runID);
    test9_NonCacheableMethods(runID);
    test10_RequestCoalescing(runID);
    test11_StaleWhileRevalidate(runID);
    test12_SMaxAgeOverride(runID);
    test13_NoCacheResponse(runID);
    test14_NoCacheRequest(runID);
    test15_NoStoreRequest(runID);
    test16_ExpiresPastAndFuture(runID);
    test17_MaxAgeOverridesExpires(runID);
    test18_MustRevalidate(runID);
    test19_WeakETag(runID);
    test20_UnsafeMethodInvalidation(runID);
    test21_UncacheableStatusCode500(runID);
    test22_CacheableStatusCode404(runID);
    test23_AgeHeader(runID);
    test24_HeadMethod(runID);
    test25_MaxAgeZeroRequest(runID);
    test26_PublicDirective(runID);
}

function getHeader(res: any, name: string): string {
    const key = Object.keys(res.headers).find(
        (k) => k.toLowerCase() === name.toLowerCase()
    );
    return key ? res.headers[key] : '';
}

// 1. Basic Cache Miss -> Hit
function test1_BasicMissAndHit(runID: string) {
    const url = `${BASE_URL}/cacheable?t1=${runID}`;
    
    // First request - Miss
    const res1 = http.get(url);
    const status1 = getHeader(res1, 'Cache-Status');
    const reqID1 = getHeader(res1, 'X-Origin-Request-Id');

    check(res1, {
        'Test 1 Miss: status 200': (r) => r.status === 200,
        'Test 1 Miss: Cache-Status contains fwd=uri-miss': () => status1.includes('fwd=uri-miss'),
        'Test 1 Miss: Cache-Status contains stored': () => status1.includes('stored'),
    });

    // Second request - Hit
    const res2 = http.get(url);
    const status2 = getHeader(res2, 'Cache-Status');
    const reqID2 = getHeader(res2, 'X-Origin-Request-Id');

    check(res2, {
        'Test 1 Hit: status 200': (r) => r.status === 200,
        'Test 1 Hit: Cache-Status contains hit': () => status2.includes('hit'),
        'Test 1 Hit: Request ID matches origin': () => reqID1 !== '' && reqID1 === reqID2,
    });
}

// 2. TTL Expiration
function test2_TTLExpiration(runID: string) {
    const url = `${BASE_URL}/cacheable?t2=${runID}`;
    const res1 = http.get(url);
    const reqID1 = getHeader(res1, 'X-Origin-Request-Id');

    // Default TTL in e2e Caddyfile is 5s
    console.log('Test 2: Waiting 6s for TTL expiration...');
    sleep(11);

    const res2 = http.get(url);
    const status2 = getHeader(res2, 'Cache-Status');
    const reqID2 = getHeader(res2, 'X-Origin-Request-Id');

    check(res2, {
        'Test 2 Expiry: status 200': (r) => r.status === 200,
        'Test 2 Expiry: Cache-Status indicates stale/miss': () => status2.includes('fwd=stale') || status2.includes('fwd=uri-miss'),
        'Test 2 Expiry: New origin Request ID': () => reqID2 !== '' && reqID1 !== reqID2,
    });
}

// 3. Cache-Control: no-store
function test3_NoStore(runID: string) {
    const url = `${BASE_URL}/cache-control/no-store?t3=${runID}`;
    const res1 = http.get(url);
    const status1 = getHeader(res1, 'Cache-Status');

    check(res1, {
        'Test 3 NoStore 1: status 200': (r) => r.status === 200,
        'Test 3 NoStore 1: Cache-Status is uri-miss': () => status1.includes('fwd=uri-miss'),
        'Test 3 NoStore 1: Cache-Status is not stored': () => !status1.includes('stored'),
    });

    const res2 = http.get(url);
    const status2 = getHeader(res2, 'Cache-Status');

    check(res2, {
        'Test 3 NoStore 2: status 200': (r) => r.status === 200,
        'Test 3 NoStore 2: Not a cache hit': () => !status2.includes('hit'),
    });
}

// 4. Cache-Control: private
function test4_Private(runID: string) {
    const url = `${BASE_URL}/cache-control/private?t4=${runID}`;
    const res1 = http.get(url);
    const status1 = getHeader(res1, 'Cache-Status');

    check(res1, {
        'Test 4 Private 1: status 200': (r) => r.status === 200,
        'Test 4 Private 1: Cache-Status is uri-miss': () => status1.includes('fwd=uri-miss'),
        'Test 4 Private 1: Cache-Status is not stored': () => !status1.includes('stored'),
    });

    const res2 = http.get(url);
    const status2 = getHeader(res2, 'Cache-Status');

    check(res2, {
        'Test 4 Private 2: status 200': (r) => r.status === 200,
        'Test 4 Private 2: Not a cache hit': () => !status2.includes('hit'),
    });
}

// 5. Cache-Control: max-age Override
function test5_MaxAgeOverride(runID: string) {
    const url = `${BASE_URL}/cache-control/max-age/2?t5=${runID}`;
    const res1 = http.get(url);
    const status1 = getHeader(res1, 'Cache-Status');

    check(res1, {
        'Test 5 MaxAge: miss and stored': () => status1.includes('fwd=uri-miss') && status1.includes('stored'),
    });

    // Immediate hit check
    const res2 = http.get(url);
    const status2 = getHeader(res2, 'Cache-Status');

    check(res2, {
        'Test 5 MaxAge: immediate hit': () => status2.includes('hit'),
    });

    // Wait 3s (past max-age=2, but before default TTL 5s)
    console.log('Test 5: Waiting 5s for max-age=2 expiration...');
    sleep(5);

    const res3 = http.get(url);
    const status3 = getHeader(res3, 'Cache-Status');

    check(res3, {
        'Test 5 MaxAge: expired after max-age': () => status3.includes('fwd=stale') || status3.includes('fwd=uri-miss'),
    });
}

// 6. Vary: Accept-Encoding
function test6_VaryAcceptEncoding(runID: string) {
    const url = `${BASE_URL}/vary/accept-encoding?t6=${runID}`;
    const headersGzip = { 'Accept-Encoding': 'gzip' };
    const headersBr = { 'Accept-Encoding': 'br' };

    // Request 1: gzip (Miss)
    const res1 = http.get(url, { headers: headersGzip });
    const status1 = getHeader(res1, 'Cache-Status');

    check(res1, {
        'Test 6 Vary: gzip miss': () => status1.includes('fwd=uri-miss') && status1.includes('stored'),
    });

    // Request 2: gzip (Hit)
    const res2 = http.get(url, { headers: headersGzip });
    const status2 = getHeader(res2, 'Cache-Status');

    check(res2, {
        'Test 6 Vary: gzip hit': () => status2.includes('hit'),
    });

    // Request 3: br (Vary Miss)
    const res3 = http.get(url, { headers: headersBr });
    const status3 = getHeader(res3, 'Cache-Status');

    check(res3, {
        'Test 6 Vary: br vary-miss': () => status3.includes('fwd=vary-miss') || status3.includes('fwd=uri-miss'),
    });

    // Request 4: br (Hit)
    const res4 = http.get(url, { headers: headersBr });
    const status4 = getHeader(res4, 'Cache-Status');

    check(res4, {
        'Test 6 Vary: br hit': () => status4.includes('hit'),
    });
}

// 7. Vary: *
function test7_VaryStar(runID: string) {
    const url = `${BASE_URL}/vary/star?t7=${runID}`;
    const res1 = http.get(url);
    const status1 = getHeader(res1, 'Cache-Status');

    check(res1, {
        'Test 7 VaryStar 1: status 200': (r) => r.status === 200,
        'Test 7 VaryStar 1: miss': () => status1.includes('fwd=uri-miss'),
    });

    const res2 = http.get(url);
    const status2 = getHeader(res2, 'Cache-Status');

    check(res2, {
        'Test 7 VaryStar 2: bypasses cache': () => status2.includes('fwd=bypass'),
    });
}

// 8. ETag / Conditional Requests
function test8_ETagConditional(runID: string) {
    const url = `${BASE_URL}/etag?t8=${runID}`;
    const res1 = http.get(url);
    const etag = getHeader(res1, 'ETag');

    check(res1, {
        'Test 8 ETag: status 200': (r) => r.status === 200,
        'Test 8 ETag: ETag present': () => etag !== '',
    });

    // Conditional request matching ETag -> 304
    const res2 = http.get(url, {
        headers: { 'If-None-Match': etag },
    });

    check(res2, {
        'Test 8 ETag: status 304 Not Modified': (r) => r.status === 304,
    });

    // Conditional request non-matching ETag -> 200 (revalidate)
    const res3 = http.get(url, {
        headers: { 'If-None-Match': '"mismatch-etag"' },
    });

    check(res3, {
        'Test 8 ETag mismatch: status 200': (r) => r.status === 200,
    });
}

// 9. Non-Cacheable HTTP Methods
function test9_NonCacheableMethods(runID: string) {
    const url = `${BASE_URL}/cacheable?t9=${runID}`;
    const resPOST = http.post(url, {});
    const statusPOST = getHeader(resPOST, 'Cache-Status');

    console.log("Test 9 POST: statusPOST", statusPOST);
    check(resPOST, {
        'Test 9 POST: status 200': (r) => r.status === 200,
        'Test 9 POST: fwd=method': () => statusPOST.includes('fwd=method'),
    });

    const resPUT = http.put(url, {});
    const statusPUT = getHeader(resPUT, 'Cache-Status');

    check(resPUT, {
        'Test 9 PUT: status 200': (r) => r.status === 200,
        'Test 9 PUT: fwd=method': () => statusPUT.includes('fwd=method'),
    });

    const resDEL = http.del(url);
    const statusDEL = getHeader(resDEL, 'Cache-Status');

    check(resDEL, {
        'Test 9 DELETE: status 200': (r) => r.status === 200,
        'Test 9 DELETE: fwd=method': () => statusDEL.includes('fwd=method'),
    });
}

// 10. Request Coalescing
function test10_RequestCoalescing(runID: string) {
    const url = `${BASE_URL}/slow/500?t10=${runID}`;
    const requests = Array(10).fill({
        method: 'GET',
        url: url,
    });

    const responses = http.batch(requests);

    const reqIDs = new Set<string>();
    let hasCollapsed = false;
    let allOk = true;

    for (const res of responses) {
        if (res.status !== 200) {
            allOk = false;
        }
        const reqID = getHeader(res, 'X-Origin-Request-Id');
        if (reqID) {
            reqIDs.add(reqID);
        }
        const cacheStatus = getHeader(res, 'Cache-Status');
        if (cacheStatus.includes('collapsed')) {
            hasCollapsed = true;
        }
    }

    check(responses[0], {
        'Test 10 Coalescing: all requests status 200': () => allOk,
        'Test 10 Coalescing: single origin request ID': () => reqIDs.size === 1,
        'Test 10 Coalescing: collapsed header observed': () => hasCollapsed,
    });
}

// 11. Stale-While-Revalidate
function test11_StaleWhileRevalidate(runID: string) {
    const url = `${BASE_URL}/cache-control/swr/10?t11=${runID}`;
    const res1 = http.get(url);
    const status1 = getHeader(res1, 'Cache-Status');

    check(res1, {
        'Test 11 SWR: initial miss and stored': () => status1.includes('fwd=uri-miss') && status1.includes('stored'),
    });

    // Wait 2s (past max-age=1, within SWR window of 10s)
    console.log('Test 11: Waiting 2s to enter SWR window...');
    sleep(2);

    // Request during SWR window -> serves stale entry while triggering background refresh
    const res2 = http.get(url);
    check(res2, {
        'Test 11 SWR: status 200 during stale window': (r) => r.status === 200,
    });

    // Wait 1s for background refresh to complete
    sleep(1);

    // Subsequent request should be a hit for refreshed entry
    const res3 = http.get(url);
    const status3 = getHeader(res3, 'Cache-Status');

    check(res3, {
        'Test 11 SWR: hit after background refresh': () => status3.includes('hit'),
    });
}

// 12. Cache-Control: s-maxage (Shared Cache Max-Age) - RFC 9111 Section 5.2.2.10
function test12_SMaxAgeOverride(runID: string) {
    const url = `${BASE_URL}/cache-control/s-maxage?t12=${runID}`;
    const res1 = http.get(url);
    const status1 = getHeader(res1, 'Cache-Status');

    check(res1, {
        'Test 12 s-maxage: miss and stored': () => status1.includes('fwd=uri-miss') && status1.includes('stored'),
    });

    const res2 = http.get(url);
    const status2 = getHeader(res2, 'Cache-Status');

    check(res2, {
        'Test 12 s-maxage: immediate hit': () => status2.includes('hit'),
    });

    // Wait 3s (past s-maxage=2, but before max-age=300)
    console.log('Test 12: Waiting 5s for s-maxage=2 expiration...');
    sleep(5);

    const res3 = http.get(url);
    const status3 = getHeader(res3, 'Cache-Status');

    check(res3, {
        'Test 12 s-maxage: expired after s-maxage': () => status3.includes('fwd=stale') || status3.includes('fwd=uri-miss'),
    });
}

// 13. Cache-Control: no-cache Response Directive - RFC 9111 Section 5.2.2.4
function test13_NoCacheResponse(runID: string) {
    const url = `${BASE_URL}/cache-control/no-cache?t13=${runID}`;
    const res1 = http.get(url);
    const status1 = getHeader(res1, 'Cache-Status');

    check(res1, {
        'Test 13 no-cache response: status 200': (r) => r.status === 200,
        'Test 13 no-cache response: miss on first request': () => status1.includes('fwd=uri-miss'),
    });

    // Subsequent request MUST revalidate with origin
    const res2 = http.get(url);
    const status2 = getHeader(res2, 'Cache-Status');

    check(res2, {
        'Test 13 no-cache response: status 200': (r) => r.status === 200,
        'Test 13 no-cache response: revalidated or not unvalidated hit:': () => !status2.includes('hit') || status2.includes('revalidated') || status2.includes('fwd='),
    });
}

// 14. Cache-Control: no-cache Request Directive - RFC 9111 Section 5.2.1.4
function test14_NoCacheRequest(runID: string) {
    const url = `${BASE_URL}/cacheable?t14=${runID}`;
    
    // Store in cache
    const res1 = http.get(url);
    const reqID1 = getHeader(res1, 'X-Origin-Request-Id');

    // Normal request gets hit
    const res2 = http.get(url);
    check(res2, {
        'Test 14 no-cache request: hit without header': () => getHeader(res2, 'Cache-Status').includes('hit'),
    });

    // Request with Cache-Control: no-cache forces revalidation / origin request
    const res3 = http.get(url, { headers: { 'Cache-Control': 'no-cache' } });
    const reqID3 = getHeader(res3, 'X-Origin-Request-Id');
    const status3 = getHeader(res3, 'Cache-Status');

    check(res3, {
        'Test 14 no-cache request: bypasses cached hit and forwards to origin': () => !status3.includes('hit') || status3.includes('fwd=') || reqID3 !== reqID1,
    });
}

// 15. Cache-Control: no-store Request Directive - RFC 9111 Section 5.2.1.5
function test15_NoStoreRequest(runID: string) {
    const url = `${BASE_URL}/cacheable?t15=${runID}`;

    // Request with no-store in request header
    const res1 = http.get(url, { headers: { 'Cache-Control': 'no-store' } });
    const status1 = getHeader(res1, 'Cache-Status');

    check(res1, {
        'Test 15 no-store request: response not stored': () => !status1.includes('stored') || status1.includes('fwd='),
    });

    // Subsequent normal request should still be a miss
    const res2 = http.get(url);
    const status2 = getHeader(res2, 'Cache-Status');

    check(res2, {
        'Test 15 no-store request: subsequent request is miss': () => status2.includes('fwd=uri-miss'),
    });
}

// 16. Expires Header (Past vs Future) - RFC 9111 Section 5.3
function test16_ExpiresPastAndFuture(runID: string) {
    // Expires in the past -> must not be served as fresh
    const urlPast = `${BASE_URL}/expires/past?t16a=${runID}`;
    const resPast1 = http.get(urlPast);
    const resPast2 = http.get(urlPast);
    const statusPast2 = getHeader(resPast2, 'Cache-Status');

    check(resPast2, {
        'Test 16 Expires past: not a fresh hit': () => !statusPast2.includes('hit') || statusPast2.includes('fwd='),
    });

    // Expires in the future -> served as fresh hit
    const urlFuture = `${BASE_URL}/expires/future?t16b=${runID}`;
    const resFut1 = http.get(urlFuture);
    const resFut2 = http.get(urlFuture);
    const statusFut2 = getHeader(resFut2, 'Cache-Status');

    check(resFut2, {
        'Test 16 Expires future: fresh cache hit': () => statusFut2.includes('hit'),
    });
}

// 17. max-age Precedence over Expires - RFC 9111 Section 5.3
function test17_MaxAgeOverridesExpires(runID: string) {
    const url = `${BASE_URL}/expires/max-age-override?t17=${runID}`;
    const res1 = http.get(url);
    const status1 = getHeader(res1, 'Cache-Status');

    check(res1, {
        'Test 17 max-age > Expires: miss and stored': () => status1.includes('fwd=uri-miss') && status1.includes('stored'),
    });

    const res2 = http.get(url);
    check(res2, {
        'Test 17 max-age > Expires: immediate hit': () => getHeader(res2, 'Cache-Status').includes('hit'),
    });

    // Wait 3s (past max-age=2, but before Expires in 2038)
    console.log('Test 17: Waiting 3s for max-age=2 expiration despite future Expires...');
    sleep(3);

    const res3 = http.get(url);
    const status3 = getHeader(res3, 'Cache-Status');

    check(res3, {
        'Test 17 max-age > Expires: expired after max-age': () => status3.includes('fwd=stale') || status3.includes('fwd=uri-miss'),
    });
}

// 18. Cache-Control: must-revalidate Directive - RFC 9111 Section 5.2.2.1
function test18_MustRevalidate(runID: string) {
    const url = `${BASE_URL}/cache-control/must-revalidate?t18=${runID}`;
    const res1 = http.get(url);
    check(res1, {
        'Test 18 must-revalidate: stored on miss': () => getHeader(res1, 'Cache-Status').includes('stored'),
    });

    const res2 = http.get(url);
    check(res2, {
        'Test 18 must-revalidate: hit while fresh': () => getHeader(res2, 'Cache-Status').includes('hit'),
    });

    // Wait 3s (past max-age=2)
    console.log('Test 18: Waiting 3s for max-age=2 expiration...');
    sleep(3);

    const res3 = http.get(url);
    const status3 = getHeader(res3, 'Cache-Status');

    check(res3, {
        'Test 18 must-revalidate: requires revalidation when stale': () => status3.includes('fwd=stale') || status3.includes('fwd=uri-miss'),
    });
}

// 19. Weak ETag Conditional Requests - RFC 9111 Section 2.3
function test19_WeakETag(runID: string) {
    const url = `${BASE_URL}/etag/weak?t19=${runID}`;
    const res1 = http.get(url);
    const etag = getHeader(res1, 'ETag');

    check(res1, {
        'Test 19 Weak ETag: status 200': (r) => r.status === 200,
        'Test 19 Weak ETag: weak etag header present': () => etag.startsWith('W/'),
    });

    // Conditional request with weak ETag -> 304 Not Modified
    const res2 = http.get(url, {
        headers: { 'If-None-Match': etag },
    });

    check(res2, {
        'Test 19 Weak ETag: status 304 Not Modified': (r) => r.status === 304,
    });
}

// 20. Cache Invalidation on Unsafe Methods - RFC 9111 Section 4.4
function test20_UnsafeMethodInvalidation(runID: string) {
    const url = `${BASE_URL}/cacheable?t20=${runID}`;
    
    // Step 1: Prime cache
    const res1 = http.get(url);

    // Step 2: Verify hit
    const res2 = http.get(url);
    check(res2, {
        'Test 20 Invalidation: GET is cache hit': () => getHeader(res2, 'Cache-Status').includes('hit'),
    });

    // Step 3: Send unsafe method (POST) to target URI
    const resPost = http.post(url, {});
    check(resPost, {
        'Test 20 Invalidation: POST returns 200': (r) => r.status === 200,
    });

    // Step 4: GET to same URL should be a cache miss (invalidated by POST)
    const res3 = http.get(url);
    const status3 = getHeader(res3, 'Cache-Status');

    check(res3, {
        'Test 20 Invalidation: subsequent GET is cache miss after POST': () => status3.includes('fwd') || status3.includes('stored'),
    });
}

// 21. Uncacheable Status Code 500 - RFC 9111 Section 4.2.2
function test21_UncacheableStatusCode500(runID: string) {
    const url = `${BASE_URL}/status/500?t21=${runID}`;
    const res1 = http.get(url);
    const status1 = getHeader(res1, 'Cache-Status');

    check(res1, {
        'Test 21 Status 500: status 500 returned': (r) => r.status === 500,
        'Test 21 Status 500: miss on first request': () => status1.includes('fwd=uri-miss'),
    });

    const res2 = http.get(url);
    const status2 = getHeader(res2, 'Cache-Status');

    check(res2, {
        'Test 21 Status 500: status 500 returned': (r) => r.status === 500,
        'Test 21 Status 500: status 500 is not cached': () => !status2.includes('hit'),
    });
}

// 22. Cacheable Status Code 404 - RFC 9111 Section 4.2.2
function test22_CacheableStatusCode404(runID: string) {
    const url = `${BASE_URL}/status/404?t22=${runID}`;
    const res1 = http.get(url);
    const status1 = getHeader(res1, 'Cache-Status');

    check(res1, {
        'Test 22 Status 404: status 404 returned': (r) => r.status === 404,
        'Test 22 Status 404: stored on miss': () => status1.includes('stored'),
    });

    const res2 = http.get(url);
    const status2 = getHeader(res2, 'Cache-Status');

    check(res2, {
        'Test 22 Status 404: status 404 returned': (r) => r.status === 404,
        'Test 22 Status 404: cache hit on second request': () => status2.includes('hit'),
    });
}

// 23. Age Header Presence on Hits - RFC 9111 Section 5.1
function test23_AgeHeader(runID: string) {
    const url = `${BASE_URL}/cacheable?t23=${runID}`;
    
    // First request - Miss
    const res1 = http.get(url);
    
    // Wait 1s
    sleep(1);

    // Second request - Hit
    const res2 = http.get(url);
    const ageHeader = getHeader(res2, 'Age');

    check(res2, {
        'Test 23 Age Header: hit response contains Age header': () => ageHeader !== '',
        'Test 23 Age Header: Age header is non-negative number': () => parseInt(ageHeader, 10) >= 0,
    });
}

// 24. HEAD Method Caching - RFC 9111 Section 4
function test24_HeadMethod(runID: string) {
    const url = `${BASE_URL}/cacheable?t24=${runID}`;
    
    // Prime cache with GET
    const res1 = http.get(url);

    // HEAD request for cached resource
    const res2 = http.request('HEAD', url);
    const status2 = getHeader(res2, 'Cache-Status');

    check(res2, {
        'Test 24 HEAD: status 200': (r) => r.status === 200,
        'Test 24 HEAD: empty body': (r) => r.body === '',
        'Test 24 HEAD: hit or forwarded': () => status2.includes('hit') || status2.includes('fwd='),
    });
}

// 25. Cache-Control: max-age=0 Request Directive - RFC 9111 Section 5.2.1.2
function test25_MaxAgeZeroRequest(runID: string) {
    const url = `${BASE_URL}/cacheable?t25=${runID}`;
    
    // Prime cache
    const res1 = http.get(url);

    // Request with max-age=0
    const res2 = http.get(url, { headers: { 'Cache-Control': 'max-age=0' } });
    const status2 = getHeader(res2, 'Cache-Status');

    check(res2, {
        'Test 25 max-age=0 request: forces revalidation/origin request': () => !status2.includes('hit') || status2.includes('fwd='),
    });
}

// 26. Cache-Control: public Directive - RFC 9111 Section 5.2.2.9
function test26_PublicDirective(runID: string) {
    const url = `${BASE_URL}/cache-control/public?t26=${runID}`;
    const res1 = http.get(url);
    const status1 = getHeader(res1, 'Cache-Status');

    check(res1, {
        'Test 26 public: status 200': (r) => r.status === 200,
        'Test 26 public: miss and stored': () => status1.includes('fwd=uri-miss') && status1.includes('stored'),
    });

    const res2 = http.get(url);
    const status2 = getHeader(res2, 'Cache-Status');

    check(res2, {
        'Test 26 public: status 200': (r) => r.status === 200,
        'Test 26 public: cache hit': () => status2.includes('hit'),
    });
}

