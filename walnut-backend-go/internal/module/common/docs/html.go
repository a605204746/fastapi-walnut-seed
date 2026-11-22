package docs

// Swagger UI / ReDoc 走 CDN（轻量托管）。url 指向本服务 /openapi.json。
// 离线环境需改为本地静态资源（Python 端用 static/swagger/ 本地 bundle）。

const swaggerHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8"/>
<meta name="viewport" content="width=device-width, initial-scale=1"/>
<title>WalnutSeed API 文档</title>
<link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5.17.14/swagger-ui.css"/>
</head>
<body>
<div id="swagger-ui"></div>
<script src="https://unpkg.com/swagger-ui-dist@5.17.14/swagger-ui-bundle.js"></script>
<script>
window.onload = () => {
  SwaggerUIBundle({
    url: "/openapi.json",
    dom_id: "#swagger-ui",
    deepLinking: true,
    presets: [SwaggerUIBundle.presets.apis, SwaggerUIBundle.SwaggerUIStandalonePreset],
    layout: "BaseLayout",
    persistAuthorization: true
  });
};
</script>
</body>
</html>`

const redocHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8"/>
<meta name="viewport" content="width=device-width, initial-scale=1"/>
<title>WalnutSeed API 文档 (ReDoc)</title>
</head>
<body>
<redoc spec-url="/openapi.json"></redoc>
<script src="https://unpkg.com/redoc@2.1.5/bundles/redoc.standalone.js"></script>
</body>
</html>`

// oauth2RedirectHTML 是 Swagger UI 标准 OAuth2 回跳页（swagger-ui-dist/oauth2-redirect.html）。
const oauth2RedirectHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head><meta charset="utf-8"/><title>Swagger UI OAuth2 Redirect</title></head>
<body>
<script>
'use strict';
function run() {
  var oauth2 = window.opener.swaggerUIRedirectOauth2;
  var sentState = oauth2.state;
  var redirectUrl = oauth2.redirectUrl;
  var isValid, qp, arr;
  if (/code|token|error/.test(window.location.hash)) {
    qp = window.location.hash.substring(1).replace('?', '&');
  } else {
    qp = location.search.substring(1);
  }
  arr = qp.split('&');
  arr.forEach(function (v, i, _arr) { _arr[i] = '"' + v.replace('=', '":"') + '"'; });
  qp = qp ? JSON.parse('{' + arr.join() + '}', function (key, value) { return key === '' ? value : decodeURIComponent(value); }) : {};
  isValid = qp.state === sentState;
  if ((oauth2.auth.schema.get('flow') === 'accessCode' || oauth2.auth.schema.get('flow') === 'authorizationCode' || oauth2.auth.schema.get('flow') === 'authorization_code') && !oauth2.code) {
    if (isValid) { oauth2.code = qp.code; oauth2.callback({ auth: oauth2.auth, redirectUrl: redirectUrl }); }
    else { oauth2.errCb({ authId: oauth2.auth.name, source: 'auth', level: 'warning', message: 'Authorization may be unsafe, passed state was changed in server.' }); }
  } else {
    oauth2.callback({ auth: oauth2.auth, token: qp, isValid: isValid, redirectUrl: redirectUrl });
  }
  window.close();
}
run();
</script>
</body>
</html>`
