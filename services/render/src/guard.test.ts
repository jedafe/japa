import { describe, expect, test } from "bun:test";
import { isPrivateIP, isPrivateLiteral, isPrivateRequestTarget, resolvesToPrivate } from "./guard.ts";

const BLOCKED = [
  "localhost",
  "foo.localhost",
  "db.internal",
  "printer.local",
  "127.0.0.1",
  "10.0.0.5",
  "172.16.3.4",
  "172.31.9.9",
  "192.168.1.1",
  "169.254.169.254",
  "100.64.0.1",
  "0.0.0.0",
  "2130706433", // decimal 127.0.0.1
  "0x7f000001", // hex 127.0.0.1
  "0177.0.0.1", // octal 127.0.0.1
  "8.8.8.8", // numeric literals refused outright (suspicious form)
  "::1",
  "fe80::1",
  "fd00::1",
  "fc00::1",
  "::ffff:127.0.0.1",
  "::ffff:10.0.0.1",
  "[::1]",
];

const ALLOWED = ["example.com", "www.walled.example", "cdn.site.co.uk"];

describe("isPrivateLiteral", () => {
  for (const h of BLOCKED) test(`blocks ${h}`, () => expect(isPrivateLiteral(h)).toBe(true));
  for (const h of ALLOWED) test(`allows ${h}`, () => expect(isPrivateLiteral(h)).toBe(false));
});

describe("isPrivateRequestTarget", () => {
  test("blocks http private literal", () =>
    expect(isPrivateRequestTarget("http://169.254.169.254/x")).toBe(true));
  test("blocks https private literal", () =>
    expect(isPrivateRequestTarget("https://127.0.0.1/")).toBe(true));
  test("allows public http", () =>
    expect(isPrivateRequestTarget("https://example.com/p")).toBe(false));
  test("data/blob pass through", () => {
    expect(isPrivateRequestTarget("data:image/png;base64,AAAA")).toBe(false);
    expect(isPrivateRequestTarget("blob:https://example.com/uuid")).toBe(false);
  });
  test("unparseable passes", () => expect(isPrivateRequestTarget("not a url")).toBe(false));
});

describe("isPrivateIP", () => {
  const priv = ["127.0.0.1", "10.1.2.3", "172.16.0.1", "172.31.255.1", "192.168.0.1",
    "169.254.1.1", "100.64.5.5", "0.0.0.0", "224.0.0.1", "255.255.255.255",
    "::1", "::", "fe80::1", "fd00::1", "fc00::1", "ff02::1", "::ffff:127.0.0.1"];
  const pub = ["8.8.8.8", "1.1.1.1", "172.32.0.1", "192.167.1.1", "100.63.0.1", "2606:4700:4700::1111"];

  for (const ip of priv) test(`private ${ip}`, () => expect(isPrivateIP(ip)).toBe(true));
  for (const ip of pub) test(`public ${ip}`, () => expect(isPrivateIP(ip)).toBe(false));
});

describe("resolvesToPrivate", () => {
  test("literal", async () => expect(await resolvesToPrivate("127.0.0.1")).toBe(true));
  test("localhost via /etc/hosts", async () => expect(await resolvesToPrivate("localhost")).toBe(true));
  test("unresolvable fails closed", async () =>
    expect(await resolvesToPrivate("nonexistent.invalid")).toBe(true));
  test("public host", async () => expect(await resolvesToPrivate("example.com")).toBe(false));
});
