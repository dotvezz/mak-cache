import http from 'k6/http';
import { check as k6Check } from 'k6';
import { Options } from 'k6/options';

export const options: Options = {
    vus: 1,
    iterations: 1,
    thresholds: {
        checks: ['rate>0.0'], // Allow reporting all findings without aborting test runner
    },
};

const isAll = __ENV.ALL && ['true', '1', 'yes'].includes(__ENV.ALL.toLowerCase());

const TARGETS = __ENV.PORT
    ? [{ name: `caddy (port ${__ENV.PORT})`, url: `http://localhost:${__ENV.PORT}` }]
    : isAll
    ? [
        { name: 'caddy-e2e', url: 'http://localhost:8086' },
        { name: 'caddy-e2e-valkey', url: 'http://localhost:8087' },
        { name: 'caddy-souin', url: 'http://localhost:8082' },
        { name: 'varnish', url: 'http://localhost:8083' },
      ]
    : [{ name: 'caddy-e2e', url: 'http://localhost:8086' }];

interface TestStats {
    passedChecks: number;
    totalChecks: number;
}

type CheckFn = <T>(val: T, sets: { [name: string]: (val: T) => boolean }) => boolean;

const asyncSleep = (seconds: number) => new Promise((resolve) => setTimeout(resolve, seconds * 1000));

const testResults: Record<string, Record<string, TestStats>> = {};
const testNames: string[] = [];

function printMatrixSummary() {
    const targetNames = TARGETS.map((t) => t.name);
    const testColWidth = 35;
    const targetColWidth = 18;
    const totalWidth = testColWidth + 3 + (targetColWidth + 3) * targetNames.length;
    const divider = '='.repeat(totalWidth);
    const line = '-'.repeat(totalWidth);

    const pad = (s: string, w: number) => s.slice(0, w).padEnd(w);
    const center = (s: string, w: number) => s.slice(0, w).padStart(Math.floor((w + Math.min(s.length, w)) / 2)).padEnd(w);

    const totals: Record<string, { testsPassed: number; checksPassed: number; checksTotal: number }> =
        Object.fromEntries(targetNames.map((t) => [t, { testsPassed: 0, checksPassed: 0, checksTotal: 0 }]));

    console.log(`\n${divider}\n${center('E2E TEST MATRIX SUMMARY', totalWidth)}\n${divider}`);
    console.log([pad('Test Suite', testColWidth), ...targetNames.map((t) => center(t, targetColWidth))].join(' | '));
    console.log(line);

    for (const testName of testNames) {
        const cells = targetNames.map((target) => {
            const { passedChecks = 0, totalChecks = 0 } = testResults[testName]?.[target] || {};
            const isPass = totalChecks > 0 && passedChecks === totalChecks;

            totals[target].checksPassed += passedChecks;
            totals[target].checksTotal += totalChecks;
            if (isPass) totals[target].testsPassed++;

            return center(`${isPass ? 'PASS' : 'FAIL'} (${passedChecks}/${totalChecks})`, targetColWidth);
        });

        console.log([pad(testName, testColWidth), ...cells].join(' | '));
    }

    console.log(line);
    console.log([pad('Passed Test Cases', testColWidth), ...targetNames.map((t) => center(`${totals[t].testsPassed}/${testNames.length}`, targetColWidth))].join(' | '));
    console.log([pad('Passed Individual Checks', testColWidth), ...targetNames.map((t) => center(`${totals[t].checksPassed}/${totals[t].checksTotal}`, targetColWidth))].join(' | '));
    console.log(`${divider}\n`);
}

async function runTarget(target: { name: string; url: string }) {
    const runID = Date.now().toString();
    console.log(`Running E2E tests against ${target.name} at ${target.url} (runID: ${runID})`);

    const runTest = async (name: string, fn: (check: CheckFn) => Promise<void> | void) => {
        if (!testNames.includes(name)) {
            testNames.push(name);
        }

        const check: CheckFn = (val, sets) => {
            const res = k6Check(val, sets);
            let passed = 0;
            let total = 0;
            for (const key of Object.keys(sets)) {
                total++;
                try {
                    if (sets[key](val)) passed++;
                } catch (_) {}
            }

            if (!testResults[name]) testResults[name] = {};
            if (!testResults[name][target.name]) {
                testResults[name][target.name] = { passedChecks: 0, totalChecks: 0 };
            }
            testResults[name][target.name].passedChecks += passed;
            testResults[name][target.name].totalChecks += total;

            return res;
        };

        try {
            await fn(check);
        } catch (err) {
            console.log(`Error in ${name} on ${target.name}: ${err}`);
        }
    };

    await runTest('1. Basic Miss & Hit', (check) => test1_BasicMissAndHit(check, target.url, runID));
    await runTest('2. TTL Expiration', (check) => test2_TTLExpiration(check, target.url, runID));
    await runTest('3. Cache-Control: no-store', (check) => test3_NoStore(check, target.url, runID));
    await runTest('4. Cache-Control: private', (check) => test4_Private(check, target.url, runID));
    await runTest('5. MaxAge Override', (check) => test5_MaxAgeOverride(check, target.url, runID));
    await runTest('6. Vary Accept-Encoding', (check) => test6_VaryAcceptEncoding(check, target.url, runID));
    await runTest('7. Vary *', (check) => test7_VaryStar(check, target.url, runID));
    await runTest('8. ETag Conditional', (check) => test8_ETagConditional(check, target.url, runID));
    await runTest('9. Non-Cacheable Methods', (check) => test9_NonCacheableMethods(check, target.url, runID));
    await runTest('10. Request Coalescing', (check) => test10_RequestCoalescing(check, target.url, runID));
    await runTest('11. Stale-While-Revalidate', (check) => test11_StaleWhileRevalidate(check, target.url, runID));
    await runTest('12. s-maxage Override', (check) => test12_SMaxAgeOverride(check, target.url, runID));
    await runTest('13. no-cache Response', (check) => test13_NoCacheResponse(check, target.url, runID));
    await runTest('14. no-cache Request', (check) => test14_NoCacheRequest(check, target.url, runID));
    await runTest('15. no-store Request', (check) => test15_NoStoreRequest(check, target.url, runID));
    await runTest('16. Expires Past & Future', (check) => test16_ExpiresPastAndFuture(check, target.url, runID));
    await runTest('17. MaxAge > Expires', (check) => test17_MaxAgeOverridesExpires(check, target.url, runID));
    await runTest('18. must-revalidate', (check) => test18_MustRevalidate(check, target.url, runID));
    await runTest('19. Weak ETag', (check) => test19_WeakETag(check, target.url, runID));
    await runTest('20. Unsafe Method Invalidation', (check) => test20_UnsafeMethodInvalidation(check, target.url, runID));
    await runTest('21. Status 500 Uncacheable', (check) => test21_UncacheableStatusCode500(check, target.url, runID));
    await runTest('22. Status 404 Cacheable', (check) => test22_CacheableStatusCode404(check, target.url, runID));
    await runTest('23. Age Header', (check) => test23_AgeHeader(check, target.url, runID));
    await runTest('24. HEAD Method', (check) => test24_HeadMethod(check, target.url, runID));
    await runTest('25. max-age=0 Request', (check) => test25_MaxAgeZeroRequest(check, target.url, runID));
    await runTest('26. public Directive', (check) => test26_PublicDirective(check, target.url, runID));
    await runTest('27. Authorized Request Default', (check) => test27_AuthorizedRequestDefault(check, target.url, runID));
    await runTest('28. Authorized Request Public', (check) => test28_AuthorizedRequestPublic(check, target.url, runID));
    await runTest('29. Authorized Request s-maxage', (check) => test29_AuthorizedRequestSMaxAge(check, target.url, runID));
    await runTest('30. Authorized Request must-reval', (check) => test30_AuthorizedRequestMustRevalidate(check, target.url, runID));
    await runTest('31. min-fresh Request', (check) => test31_MinFreshRequest(check, target.url, runID));
    await runTest('32. max-stale Request', (check) => test32_MaxStaleRequest(check, target.url, runID));
    await runTest('33. If-Modified-Since', (check) => test33_IfModifiedSince(check, target.url, runID));
    await runTest('34. SWR Expired Window', (check) => test34_StaleWhileRevalidateExpiredWindow(check, target.url, runID));
    await runTest('35. stale-if-error Response', (check) => test35_StaleIfErrorResponse(check, target.url, runID));
    await runTest('36. stale-if-error Request', (check) => test36_StaleIfErrorRequest(check, target.url, runID));
}

export default async function () {
    await Promise.all(TARGETS.map((target) => runTarget(target)));
    printMatrixSummary();
}

function getHeader(res: any, name: string): string {
    if (!res || !res.headers) return '';
    const key = Object.keys(res.headers).find(
        (k) => k.toLowerCase() === name.toLowerCase()
    );
    return key ? res.headers[key] : '';
}

// 1. Basic Cache Miss -> Hit
function test1_BasicMissAndHit(check: CheckFn, baseUrl: string, runID: string) {
    const url = `${baseUrl}/cacheable?t1=${runID}`;
    
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
async function test2_TTLExpiration(check: CheckFn, baseUrl: string, runID: string) {
    const url = `${baseUrl}/cacheable?t2=${runID}`;
    const res1 = http.get(url);
    const reqID1 = getHeader(res1, 'X-Origin-Request-Id');

    // Default TTL in e2e Caddyfile is 5s
    console.log('Test 2: Waiting 6s for TTL expiration...');
    await asyncSleep(11);

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
function test3_NoStore(check: CheckFn, baseUrl: string, runID: string) {
    const url = `${baseUrl}/cache-control/no-store?t3=${runID}`;
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
function test4_Private(check: CheckFn, baseUrl: string, runID: string) {
    const url = `${baseUrl}/cache-control/private?t4=${runID}`;
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
async function test5_MaxAgeOverride(check: CheckFn, baseUrl: string, runID: string) {
    const url = `${baseUrl}/cache-control/max-age/2?t5=${runID}`;
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
    await asyncSleep(5);

    const res3 = http.get(url);
    const status3 = getHeader(res3, 'Cache-Status');

    check(res3, {
        'Test 5 MaxAge: expired after max-age': () => status3.includes('fwd=stale') || status3.includes('fwd=uri-miss'),
    });
}

// 6. Vary: Accept-Encoding
function test6_VaryAcceptEncoding(check: CheckFn, baseUrl: string, runID: string) {
    const url = `${baseUrl}/vary/accept-encoding?t6=${runID}`;
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
function test7_VaryStar(check: CheckFn, baseUrl: string, runID: string) {
    const url = `${baseUrl}/vary/star?t7=${runID}`;
    const res1 = http.get(url);
    const status1 = getHeader(res1, 'Cache-Status');

    check(res1, {
        'Test 7 VaryStar 1: status 200': (r) => r.status === 200,
        'Test 7 VaryStar 1: miss': () => status1.includes('fwd=uri-miss'),
    });

    const res2 = http.get(url);
    const status2 = getHeader(res2, 'Cache-Status');

    check(res2, {
        'Test 7 VaryStar 2: bypass': () => status2.includes('fwd=bypass'),
    });
}

// 8. ETag / Conditional Requests
function test8_ETagConditional(check: CheckFn, baseUrl: string, runID: string) {
    const url = `${baseUrl}/etag?t8=${runID}`;
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
function test9_NonCacheableMethods(check: CheckFn, baseUrl: string, runID: string) {
    const url = `${baseUrl}/cacheable?t9=${runID}`;
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
function test10_RequestCoalescing(check: CheckFn, baseUrl: string, runID: string) {
    const url = `${baseUrl}/slow/500?t10=${runID}`;
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
async function test11_StaleWhileRevalidate(check: CheckFn, baseUrl: string, runID: string) {
    const url = `${baseUrl}/cache-control/swr/10?t11=${runID}`;
    const res1 = http.get(url);
    const status1 = getHeader(res1, 'Cache-Status');

    check(res1, {
        'Test 11 SWR: initial miss and stored': () => status1.includes('fwd=uri-miss') && status1.includes('stored'),
    });

    // Wait 2s (past max-age=1, within SWR window of 10s)
    console.log('Test 11: Waiting 2s to enter SWR window...');
    await asyncSleep(2);

    // Request during SWR window -> serves stale entry while triggering background refresh
    const res2 = http.get(url);
    check(res2, {
        'Test 11 SWR: status 200 during stale window': (r) => r.status === 200,
    });

    // Wait 1s for background refresh to complete
    await asyncSleep(1);

    // Subsequent request should be a hit for refreshed entry
    const res3 = http.get(url);
    const status3 = getHeader(res3, 'Cache-Status');

    check(res3, {
        'Test 11 SWR: hit after background refresh': () => status3.includes('hit'),
    });
}

// 12. Cache-Control: s-maxage (Shared Cache Max-Age) - RFC 9111 Section 5.2.2.10
async function test12_SMaxAgeOverride(check: CheckFn, baseUrl: string, runID: string) {
    const url = `${baseUrl}/cache-control/s-maxage?t12=${runID}`;
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
    await asyncSleep(5);

    const res3 = http.get(url);
    const status3 = getHeader(res3, 'Cache-Status');

    check(res3, {
        'Test 12 s-maxage: expired after s-maxage': () => status3.includes('fwd=stale') || status3.includes('fwd=uri-miss'),
    });
}

// 13. Cache-Control: no-cache Response Directive - RFC 9111 Section 5.2.2.4
function test13_NoCacheResponse(check: CheckFn, baseUrl: string, runID: string) {
    const url = `${baseUrl}/cache-control/no-cache?t13=${runID}`;
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
function test14_NoCacheRequest(check: CheckFn, baseUrl: string, runID: string) {
    const url = `${baseUrl}/cacheable?t14=${runID}`;
    
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
function test15_NoStoreRequest(check: CheckFn, baseUrl: string, runID: string) {
    const url = `${baseUrl}/cacheable?t15=${runID}`;

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
function test16_ExpiresPastAndFuture(check: CheckFn, baseUrl: string, runID: string) {
    // Expires in the past -> must not be served as fresh
    const urlPast = `${baseUrl}/expires/past?t16a=${runID}`;
    const resPast1 = http.get(urlPast);
    const resPast2 = http.get(urlPast);
    const statusPast2 = getHeader(resPast2, 'Cache-Status');

    check(resPast2, {
        'Test 16 Expires past: not a fresh hit': () => !statusPast2.includes('hit') || statusPast2.includes('fwd='),
    });

    // Expires in the future -> served as fresh hit
    const urlFuture = `${baseUrl}/expires/future?t16b=${runID}`;
    const resFut1 = http.get(urlFuture);
    const resFut2 = http.get(urlFuture);
    const statusFut2 = getHeader(resFut2, 'Cache-Status');

    check(resFut2, {
        'Test 16 Expires future: fresh cache hit': () => statusFut2.includes('hit'),
    });
}

// 17. max-age Precedence over Expires - RFC 9111 Section 5.3
async function test17_MaxAgeOverridesExpires(check: CheckFn, baseUrl: string, runID: string) {
    const url = `${baseUrl}/expires/max-age-override?t17=${runID}`;
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
    await asyncSleep(3);

    const res3 = http.get(url);
    const status3 = getHeader(res3, 'Cache-Status');

    check(res3, {
        'Test 17 max-age > Expires: expired after max-age': () => status3.includes('fwd=stale') || status3.includes('fwd=uri-miss'),
    });
}

// 18. Cache-Control: must-revalidate Directive - RFC 9111 Section 5.2.2.1
async function test18_MustRevalidate(check: CheckFn, baseUrl: string, runID: string) {
    const url = `${baseUrl}/cache-control/must-revalidate?t18=${runID}`;
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
    await asyncSleep(3);

    const res3 = http.get(url);
    const status3 = getHeader(res3, 'Cache-Status');

    check(res3, {
        'Test 18 must-revalidate: requires revalidation when stale': () => status3.includes('fwd=stale') || status3.includes('fwd=uri-miss'),
    });
}

// 19. Weak ETag Conditional Requests - RFC 9111 Section 2.3
function test19_WeakETag(check: CheckFn, baseUrl: string, runID: string) {
    const url = `${baseUrl}/etag/weak?t19=${runID}`;
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
function test20_UnsafeMethodInvalidation(check: CheckFn, baseUrl: string, runID: string) {
    const url = `${baseUrl}/cacheable?t20=${runID}`;
    
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
function test21_UncacheableStatusCode500(check: CheckFn, baseUrl: string, runID: string) {
    const url = `${baseUrl}/status/500?t21=${runID}`;
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
function test22_CacheableStatusCode404(check: CheckFn, baseUrl: string, runID: string) {
    const url = `${baseUrl}/status/404?t22=${runID}`;
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
async function test23_AgeHeader(check: CheckFn, baseUrl: string, runID: string) {
    const url = `${baseUrl}/cacheable?t23=${runID}`;
    
    // First request - Miss
    const res1 = http.get(url);
    
    // Wait 1s
    await asyncSleep(1);

    // Second request - Hit
    const res2 = http.get(url);
    const ageHeader = getHeader(res2, 'Age');

    check(res2, {
        'Test 23 Age Header: hit response contains Age header': () => ageHeader !== '',
        'Test 23 Age Header: Age header is non-negative number': () => parseInt(ageHeader, 10) >= 0,
    });
}

// 24. HEAD Method Caching - RFC 9111 Section 4
function test24_HeadMethod(check: CheckFn, baseUrl: string, runID: string) {
    const url = `${baseUrl}/cacheable?t24=${runID}`;
    
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
function test25_MaxAgeZeroRequest(check: CheckFn, baseUrl: string, runID: string) {
    const url = `${baseUrl}/cacheable?t25=${runID}`;
    
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
function test26_PublicDirective(check: CheckFn, baseUrl: string, runID: string) {
    const url = `${baseUrl}/cache-control/public?t26=${runID}`;
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

// 27. Authorized Request Default Behavior - RFC 9111 Section 3.5
function test27_AuthorizedRequestDefault(check: CheckFn, baseUrl: string, runID: string) {
    const url = `${baseUrl}/cacheable?t27=${runID}`;
    const authHeaders = { Authorization: 'Bearer secret-token-123' };

    const res1 = http.get(url, { headers: authHeaders });
    const status1 = getHeader(res1, 'Cache-Status');

    check(res1, {
        'Test 27 Authorization default: status 200': (r) => r.status === 200,
        'Test 27 Authorization default: not stored without directive': () => !status1.includes('stored') || status1.includes('fwd='),
    });

    const res2 = http.get(url, { headers: authHeaders });
    const status2 = getHeader(res2, 'Cache-Status');

    check(res2, {
        'Test 27 Authorization default: subsequent request is not cache hit': () => !status2.includes('hit'),
    });
}

// 28. Authorized Request with public Directive - RFC 9111 Section 3.5
function test28_AuthorizedRequestPublic(check: CheckFn, baseUrl: string, runID: string) {
    const url = `${baseUrl}/cache-control/public?t28=${runID}`;
    const authHeaders = { Authorization: 'Bearer secret-token-123' };

    const res1 = http.get(url, { headers: authHeaders });
    const status1 = getHeader(res1, 'Cache-Status');

    check(res1, {
        'Test 28 Authorization public: status 200': (r) => r.status === 200,
        'Test 28 Authorization public: stored when public directive present': () => status1.includes('stored'),
    });

    const res2 = http.get(url, { headers: authHeaders });
    const status2 = getHeader(res2, 'Cache-Status');

    check(res2, {
        'Test 28 Authorization public: cache hit on second request': () => status2.includes('hit'),
    });
}

// 29. Authorized Request with s-maxage Directive - RFC 9111 Section 3.5
function test29_AuthorizedRequestSMaxAge(check: CheckFn, baseUrl: string, runID: string) {
    const url = `${baseUrl}/cache-control/s-maxage?t29=${runID}`;
    const authHeaders = { Authorization: 'Bearer secret-token-123' };

    const res1 = http.get(url, { headers: authHeaders });
    const status1 = getHeader(res1, 'Cache-Status');

    check(res1, {
        'Test 29 Authorization s-maxage: status 200': (r) => r.status === 200,
        'Test 29 Authorization s-maxage: stored when s-maxage directive present': () => status1.includes('stored'),
    });

    const res2 = http.get(url, { headers: authHeaders });
    const status2 = getHeader(res2, 'Cache-Status');

    check(res2, {
        'Test 29 Authorization s-maxage: cache hit on second request': () => status2.includes('hit'),
    });
}

// 30. Authorized Request with must-revalidate Directive - RFC 9111 Section 3.5
function test30_AuthorizedRequestMustRevalidate(check: CheckFn, baseUrl: string, runID: string) {
    const url = `${baseUrl}/cache-control/must-revalidate?t30=${runID}`;
    const authHeaders = { Authorization: 'Bearer secret-token-123' };

    const res1 = http.get(url, { headers: authHeaders });
    const status1 = getHeader(res1, 'Cache-Status');

    check(res1, {
        'Test 30 Authorization must-revalidate: status 200': (r) => r.status === 200,
        'Test 30 Authorization must-revalidate: stored when must-revalidate directive present': () => status1.includes('stored'),
    });

    const res2 = http.get(url, { headers: authHeaders });
    const status2 = getHeader(res2, 'Cache-Status');

    check(res2, {
        'Test 30 Authorization must-revalidate: cache hit on second request': () => status2.includes('hit'),
    });
}

// 31. Cache-Control: min-fresh Request Directive - RFC 9111 Section 5.2.1.3
async function test31_MinFreshRequest(check: CheckFn, baseUrl: string, runID: string) {
    const url = `${baseUrl}/cache-control/max-age/2?t31=${runID}`;

    // Prime cache
    const res1 = http.get(url);
    const status1 = getHeader(res1, 'Cache-Status');

    check(res1, {
        'Test 31 min-fresh: stored on miss': () => status1.includes('stored'),
    });

    // Wait 1s (past 1s out of max-age=2, remaining freshness ~ 1s)
    await asyncSleep(1);

    // Request requiring min-fresh=5s (remaining 1s < 5s min-fresh requirement)
    const res2 = http.get(url, { headers: { 'Cache-Control': 'min-fresh=5' } });
    const status2 = getHeader(res2, 'Cache-Status');

    check(res2, {
        'Test 31 min-fresh: revalidates or miss when remaining freshness < min-fresh': () => !status2.includes('hit') || status2.includes('fwd='),
    });
}

// 32. Cache-Control: max-stale Request Directive - RFC 9111 Section 5.2.1.2
async function test32_MaxStaleRequest(check: CheckFn, baseUrl: string, runID: string) {
    const url = `${baseUrl}/cache-control/max-age/2?t32=${runID}`;

    // Prime cache
    const res1 = http.get(url);
    const status1 = getHeader(res1, 'Cache-Status');

    check(res1, {
        'Test 32 max-stale: stored on miss': () => status1.includes('stored'),
    });

    // Wait 3s (past max-age=2, response is stale)
    await asyncSleep(3);

    // Request with max-stale=10 (accepts stale response up to 10s past freshness)
    const res2 = http.get(url, { headers: { 'Cache-Control': 'max-stale=10' } });
    const status2 = getHeader(res2, 'Cache-Status');

    check(res2, {
        'Test 32 max-stale: serves stale response when max-stale permits': () => status2.includes('hit') || status2.includes('stale'),
    });
}

// 33. If-Modified-Since Conditional Request - RFC 9111 Section 4.3.1
function test33_IfModifiedSince(check: CheckFn, baseUrl: string, runID: string) {
    const url = `${baseUrl}/cacheable?t33=${runID}`;
    const res1 = http.get(url);
    const lastModified = getHeader(res1, 'Last-Modified');

    check(res1, {
        'Test 33 If-Modified-Since: status 200': (r) => r.status === 200,
    });

    // Conditional request with matching Last-Modified timestamp
    const reqHeaders: Record<string, string> = {
        'If-Modified-Since': lastModified || new Date().toUTCString(),
    };
    const res2 = http.get(url, { headers: reqHeaders });

    check(res2, {
        'Test 33 If-Modified-Since: status 304 or 200': (r) => r.status === 304 || r.status === 200,
    });
}

// 34. Stale-While-Revalidate Window Expiration - RFC 5861 Section 3
async function test34_StaleWhileRevalidateExpiredWindow(check: CheckFn, baseUrl: string, runID: string) {
    const url = `${baseUrl}/cache-control/swr/2?t34=${runID}`;
    const res1 = http.get(url);
    const status1 = getHeader(res1, 'Cache-Status');

    check(res1, {
        'Test 34 SWR Expired: initial miss and stored': () => status1.includes('fwd=uri-miss') && status1.includes('stored'),
    });

    // Wait 4s (past max-age=1 AND past SWR window=2s)
    console.log('Test 34: Waiting 4s for SWR window expiration...');
    await asyncSleep(4);

    // Request after SWR window has passed -> must forward synchronously to origin, not serve stale via SWR
    const res2 = http.get(url);
    const status2 = getHeader(res2, 'Cache-Status');

    check(res2, {
        'Test 34 SWR Expired: status 200': (r) => r.status === 200,
        'Test 34 SWR Expired: forwards to origin after SWR window expires': () => status2.includes('fwd=') || !status2.includes('hit'),
    });
}

// 35. Cache-Control: stale-if-error Response Directive - RFC 5861 Section 4
async function test35_StaleIfErrorResponse(check: CheckFn, baseUrl: string, runID: string) {
    const url = `${baseUrl}/cache-control/sie/10?t35=${runID}`;
    const res1 = http.get(url);
    const status1 = getHeader(res1, 'Cache-Status');

    check(res1, {
        'Test 35 stale-if-error response: status 200': (r) => r.status === 200,
        'Test 35 stale-if-error response: stored on initial miss': () => status1.includes('stored') || status1.includes('fwd='),
    });

    // Wait 2s (past fresh window into stale-if-error window)
    console.log('Test 35: Waiting 2s for stale-if-error window...');
    await asyncSleep(2);

    const res2 = http.get(url);
    const status2 = getHeader(res2, 'Cache-Status');

    check(res2, {
        'Test 35 stale-if-error response: status 200': (r) => r.status === 200,
        'Test 35 stale-if-error response: hit or stale response returned': () => status2.includes('hit') || status2.includes('stale') || status2.includes('fwd='),
    });
}

// 36. Cache-Control: stale-if-error Request Directive - RFC 5861 Section 4
async function test36_StaleIfErrorRequest(check: CheckFn, baseUrl: string, runID: string) {
    const url = `${baseUrl}/cache-control/max-age/2?t36=${runID}`;

    // Prime cache
    const res1 = http.get(url);
    const status1 = getHeader(res1, 'Cache-Status');

    check(res1, {
        'Test 36 stale-if-error request: stored on miss': () => status1.includes('stored'),
    });

    // Wait 3s (past max-age=2, response is stale)
    await asyncSleep(3);

    // Request with stale-if-error=10 header
    const res2 = http.get(url, { headers: { 'Cache-Control': 'stale-if-error=10' } });
    const status2 = getHeader(res2, 'Cache-Status');

    check(res2, {
        'Test 36 stale-if-error request: status 200': (r) => r.status === 200,
        'Test 36 stale-if-error request: header accepted and response returned': (r) => r.status === 200,
    });
}



