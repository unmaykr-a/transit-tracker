// A minimal OpenID provider for the e2e tests (no dependencies): discovery,
// JWKS, an authorize endpoint that approves at once as the user chosen with
// GET /as?sub=…, and a token endpoint that checks PKCE and returns an RS256 ID token.
import {
  createHash,
  generateKeyPairSync,
  randomBytes,
  sign,
} from "node:crypto";
import { createServer } from "node:http";

const port = Number(process.env.IDP_PORT ?? 4174);
const issuer = `http://127.0.0.1:${port}`;
const { publicKey, privateKey } = generateKeyPairSync("rsa", {
  modulusLength: 2048,
});
const jwk = {
  ...publicKey.export({ format: "jwk" }),
  kid: "k1",
  alg: "RS256",
  use: "sig",
};
const codes = new Map(); // code → { claims, challenge }
const b64 = (b) => Buffer.from(b).toString("base64url");

function idToken(claims) {
  const head = b64(JSON.stringify({ alg: "RS256", typ: "JWT", kid: "k1" }));
  const body = b64(JSON.stringify(claims));
  return `${head}.${body}.${b64(sign("sha256", Buffer.from(`${head}.${body}`), privateKey))}`;
}

function json(res, status, v) {
  res.writeHead(status, { "Content-Type": "application/json" });
  res.end(JSON.stringify(v));
}

createServer((req, res) => {
  const url = new URL(req.url, issuer);
  if (url.pathname === "/.well-known/openid-configuration") {
    return json(res, 200, {
      issuer,
      authorization_endpoint: `${issuer}/authorize`,
      token_endpoint: `${issuer}/token`,
      jwks_uri: `${issuer}/keys`,
      id_token_signing_alg_values_supported: ["RS256"],
    });
  }
  if (url.pathname === "/keys") return json(res, 200, { keys: [jwk] });
  if (url.pathname === "/healthz") return json(res, 200, { ok: true });
  if (url.pathname === "/as") {
    // "Signs in" at the provider: the browser's cookie names the user it approves as.
    res.writeHead(200, {
      "Set-Cookie": `idp_sub=${encodeURIComponent(url.searchParams.get("sub") ?? "")}; Path=/authorize; SameSite=Lax`,
      "Content-Type": "text/plain",
    });
    return res.end("ok");
  }
  if (url.pathname === "/authorize") {
    const sub = decodeURIComponent(
      /(?:^|; )idp_sub=([^;]*)/.exec(req.headers.cookie ?? "")?.[1] ?? "",
    );
    const q = url.searchParams;
    if (!sub) return json(res, 401, { error: "not signed in at the provider" });
    const now = Math.floor(Date.now() / 1000);
    const code = randomBytes(16).toString("hex");
    codes.set(code, {
      challenge: q.get("code_challenge"),
      claims: {
        iss: issuer,
        aud: q.get("client_id"),
        sub,
        nonce: q.get("nonce"),
        iat: now,
        exp: now + 300,
        preferred_username: sub,
        email: `${sub}@example.com`,
        email_verified: true,
      },
    });
    res.writeHead(302, {
      Location: `${q.get("redirect_uri")}?state=${encodeURIComponent(q.get("state") ?? "")}&code=${code}`,
    });
    return res.end();
  }
  if (url.pathname === "/token" && req.method === "POST") {
    let body = "";
    req.on("data", (c) => (body += c));
    req.on("end", () => {
      const form = new URLSearchParams(body);
      const grant = codes.get(form.get("code"));
      codes.delete(form.get("code"));
      const verifier = createHash("sha256")
        .update(form.get("code_verifier") ?? "")
        .digest("base64url");
      if (!grant || verifier !== grant.challenge)
        return json(res, 400, { error: "invalid_grant" });
      json(res, 200, {
        access_token: "at",
        token_type: "Bearer",
        expires_in: 300,
        id_token: idToken(grant.claims),
      });
    });
    return;
  }
  json(res, 404, { error: "not found" });
}).listen(port, "127.0.0.1");
