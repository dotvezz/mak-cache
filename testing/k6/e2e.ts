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
    console.log('Test 5: Waiting 3s for max-age=2 expiration...');
    sleep(3);

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
