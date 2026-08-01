import http from 'k6/http';
import {check, sleep} from 'k6';
import {Options} from 'k6/options';

export const options: Options = {
    thresholds: {
        http_req_duration: ['p(95)<1000'],
    },
    scenarios: {
        oneLargeJSON: {
            executor: 'ramping-vus',
            exec: 'oneLargeJSON',
            stages: [
                {duration: '5s', target: 200},
                {duration: '5s', target: 2000},
                {duration: '5s', target: 5000},
                {duration: '5s', target: 10000},
                {duration: '5s', target: 12000},
                {duration: '5s', target: 15000},
                {duration: '5s', target: 5000},
            ]
        },
    },
};

export const oneLargeJSON = () => {
    const res = getPath(8080, "large.json");

    check(res, {
        'status is 200': (r) => r.status === 200,
    });

    sleep(1);
}

function getPath(port: number, id: string) {
    return http.get(`http://localhost:${port}/${id}`);
}