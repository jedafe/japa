// Wall-detector regression tests — runs the service's signature port
// against the same fixture corpus the Go detector is tested with
// (internal/intelligence/fetcher/testdata/). If vendor markup drifts or
// the two ports diverge, both sides flag it.
import { describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { detectWall } from "./wall.ts";

const FIXTURES = "../../../internal/intelligence/fetcher/testdata/";

const cases: [string, string | null][] = [
  ["wall_cloudflare.html", "cloudflare-challenge"],
  ["wall_datadome.html", "datadome-captcha"],
  ["wall_perimeterx.html", "perimeterx"],
  ["wall_incapsula.html", "incapsula-denied"],
  ["wall_akamai.html", "akamai-denied"],
  ["legit_thin.html", null],
];

describe("detectWall fixtures", () => {
  for (const [file, want] of cases) {
    test(file, () => {
      const html = readFileSync(new URL(FIXTURES + file, import.meta.url), "utf8");
      expect(detectWall(html)).toBe(want);
    });
  }

  test("empty body", () => expect(detectWall("")).toBeNull());
});
