import http from "k6/http";
import { check } from "k6";

export const options = {
  scenarios: {
    redirects: {
      executor: "constant-arrival-rate",
      rate: Number(__ENV.RATE || 1000),
      timeUnit: "1s",
      duration: __ENV.DURATION || "60s",
      preAllocatedVUs: 100,
      maxVUs: 500,
    },
  },
  thresholds: {
    http_req_duration: ["p(95)<50"],
    http_req_failed: ["rate<0.01"],
  },
};

export default function () {
  const response = http.get(`${__ENV.BASE_URL || "http://localhost:8081"}/${__ENV.ALIAS || "launch"}`, { redirects: 0 });
  check(response, { "redirected": (r) => r.status === 301 || r.status === 302 });
}
