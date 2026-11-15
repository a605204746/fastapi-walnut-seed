import { generateKeyPairSync } from 'node:crypto';

import { describe, expect, it } from 'vitest';

import {
  AesEncryption,
  decodeBase64,
  encodeBase64,
  randomStr,
  RsaEncryption,
} from '../index';

/**
 * 生成裸 base64 DER 密钥（无 PEM 头），与项目 .env 中
 * VITE_GLOB_RSA_PUBLIC_KEY / VITE_GLOB_RSA_PRIVATE_KEY 的实际格式一致。
 */
function generateKeyPair(modulusLength = 1024) {
  const { privateKey, publicKey } = generateKeyPairSync('rsa', {
    modulusLength,
    privateKeyEncoding: { format: 'der', type: 'pkcs8' },
    publicKeyEncoding: { format: 'der', type: 'spki' },
  });
  return {
    privateKey: privateKey.toString('base64'),
    publicKey: publicKey.toString('base64'),
  };
}

describe('crypto 基础工具', () => {
  it('randomStr 默认生成 32 位', () => {
    expect(randomStr()).toHaveLength(32);
    expect(randomStr(16)).toHaveLength(16);
  });

  it('randomStr 只使用 base62 字符集（AES-256 密钥要求）', () => {
    for (let i = 0; i < 50; i++) {
      expect(randomStr(32)).toMatch(/^[0-9a-zA-Z]{32}$/);
    }
  });

  it('randomStr 两次调用结果不同', () => {
    expect(randomStr(32)).not.toBe(randomStr(32));
  });

  it('base64 编解码可逆', () => {
    expect(decodeBase64(encodeBase64('hello'))).toBe('hello');
    expect(decodeBase64(encodeBase64(randomStr(32)))).toHaveLength(32);
  });

  it('base64 编解码支持中文与 emoji', () => {
    const raw = ' WalnutSeed 核桃 🌰 ';
    expect(decodeBase64(encodeBase64(raw))).toBe(raw);
  });
});

describe('AesEncryption (ECB + PkCS7)', () => {
  const aes = new AesEncryption();

  it('加解密可逆', () => {
    const key = randomStr(32);
    const cipher = aes.encrypt('plain text', key);
    expect(aes.decrypt(cipher, key)).toBe('plain text');
  });

  it('JSON 载荷加解密可逆', () => {
    const key = randomStr(32);
    const payload = {
      arr: [1, '2', { nested: true }],
      empty: null,
      n: 123_456_789,
      s: '包含中文与符号 !@#$%^&*()',
    };
    const cipher = aes.encrypt(JSON.stringify(payload), key);
    expect(JSON.parse(aes.decrypt(cipher, key))).toEqual(payload);
  });

  it('密文是 base64，不含明文片段', () => {
    const key = randomStr(32);
    const cipher = aes.encrypt('username=admin&password=secret', key);
    expect(cipher).toMatch(/^[A-Za-z0-9+/]+=*$/);
    expect(cipher).not.toContain('password');
  });

  it('ECB 模式下同 key 同明文得到同密文（锁定分组模式，改成 CBC 会破坏后端兼容）', () => {
    const key = randomStr(32);
    expect(aes.encrypt('same input', key)).toBe(aes.encrypt('same input', key));
  });

  it('不同 key 无法解密（密钥不匹配时要么抛错要么得到乱码）', () => {
    const cipher = aes.encrypt('secret', randomStr(32));
    let wrong: string | undefined;
    let threw = false;
    try {
      wrong = aes.decrypt(cipher, randomStr(32));
    } catch {
      // crypto-js 解出的乱码若不是合法 UTF-8，toString(Utf8) 会抛
      // "Malformed UTF-8 data"。乱码偶然合法时则不抛，故两种结果都要接受。
      threw = true;
    }
    expect(threw || wrong !== 'secret').toBe(true);
  });

  it('空字符串可逆', () => {
    const key = randomStr(32);
    expect(aes.decrypt(aes.encrypt('', key), key)).toBe('');
  });
});

describe('RsaEncryption', () => {
  const keyPair = generateKeyPair();
  const rsa = new RsaEncryption(keyPair);

  it('裸 base64 DER 密钥（无 PEM 头）可用', () => {
    expect(keyPair.publicKey).not.toContain('-----BEGIN');
    expect(rsa.decrypt(rsa.encrypt('hello'))).toBe('hello');
  });

  it('可加解密 44 字符的 base64 AES 密钥', () => {
    const keyWithBase64 = encodeBase64(randomStr(32));
    expect(keyWithBase64).toHaveLength(44);
    expect(rsa.decrypt(rsa.encrypt(keyWithBase64))).toBe(keyWithBase64);
  });

  it('公私钥分离：公钥加密的内容只能用私钥解开', () => {
    const encryptor = new RsaEncryption({
      privateKey: '',
      publicKey: keyPair.publicKey,
    });
    const decryptor = new RsaEncryption({
      privateKey: keyPair.privateKey,
      publicKey: '',
    });
    const cipher = encryptor.encrypt('cross instance');
    expect(decryptor.decrypt(cipher)).toBe('cross instance');
  });

  it('错误的私钥解密抛错而不是静默返回', () => {
    const other = generateKeyPair();
    const cipher = rsa.encrypt('payload');
    const stranger = new RsaEncryption({
      privateKey: other.privateKey,
      publicKey: other.publicKey,
    });
    expect(() => stranger.decrypt(cipher)).toThrow(
      'RsaEncryption decrypt error',
    );
  });
});

/**
 * 端到端契约测试：复刻 apps/web-react 请求层的加解密装配，
 * 与三个后端（FastAPI / Spring Boot / Gin）约定的线格式保持一致。
 *
 * 密钥布局（两对 RSA）：
 *   请求方向 —— 前端持 publicKey 加密，后端持对应 privateKey 解密
 *   响应方向 —— 后端持 publicKey 加密，前端持对应 privateKey 解密
 */
describe('接口加解密信封（前后端契约）', () => {
  const requestKeys = generateKeyPair();
  const responseKeys = generateKeyPair();

  // 前端：请求加密公钥 + 响应解密私钥
  const frontend = new RsaEncryption({
    privateKey: responseKeys.privateKey,
    publicKey: requestKeys.publicKey,
  });
  // 后端：请求解密私钥 + 响应加密公钥
  const backend = new RsaEncryption({
    privateKey: requestKeys.privateKey,
    publicKey: responseKeys.publicKey,
  });
  const aes = new AesEncryption();

  it('请求方向：encrypt-key 头 + AES body 可被后端还原', () => {
    const payload = {
      deptId: 103,
      pageNum: 1,
      pageSize: 10,
      username: 'admin',
    };

    // ---- 前端发送 ----
    const key = randomStr(32);
    const keyWithBase64 = encodeBase64(key);
    const headers: Record<string, string> = {
      'encrypt-key': frontend.encrypt(keyWithBase64),
    };
    const body = aes.encrypt(JSON.stringify(payload), key);

    // ---- 后端接收 ----
    const base64Str = backend.decrypt(headers['encrypt-key']!);
    const secret = decodeBase64(base64Str);
    expect(secret).toBe(key);
    expect(JSON.parse(aes.decrypt(body, secret))).toEqual(payload);
  });

  it('响应方向：后端另生成一次性密钥、用响应公钥加密后放进响应头', () => {
    const envelope = {
      code: 200,
      data: { total: 2, rows: [{ userId: 1 }, { userId: 2 }] },
      msg: '操作成功',
    };

    // ---- 前端发起请求 ----
    const requestKey = randomStr(32);
    const requestHeader = frontend.encrypt(encodeBase64(requestKey));

    // ---- 后端解密请求 ----
    const backendSecret = decodeBase64(backend.decrypt(requestHeader));
    expect(backendSecret).toBe(requestKey);

    // ---- 后端加密响应 ----
    // 响应不复用请求的 AES 密钥：后端另生成一次性密钥，
    // 用「响应公钥」加密后放进响应的 encrypt-key 头。
    const responseKey = randomStr(32);
    const responseHeader = backend.encrypt(encodeBase64(responseKey));
    const responseBody = aes.encrypt(JSON.stringify(envelope), responseKey);

    // ---- 前端解密响应 ----
    const clientSecret = decodeBase64(frontend.decrypt(responseHeader));
    expect(clientSecret).toBe(responseKey);

    const decrypted = JSON.parse(aes.decrypt(responseBody, clientSecret));
    expect(decrypted).toEqual(envelope);
    expect(decrypted).toMatchObject({
      code: 200,
      data: { total: 2 },
      msg: '操作成功',
    });
  });

  it('两对密钥相互独立：前端无法用自己的私钥解开请求头', () => {
    const requestHeader = frontend.encrypt(encodeBase64(randomStr(32)));
    // 前端只持有「请求公钥」与「响应私钥」，解不开自己加密的请求头
    expect(() => frontend.decrypt(requestHeader)).toThrow(
      'RsaEncryption decrypt error',
    );
    // 后端持有「请求私钥」，可以解开
    expect(decodeBase64(backend.decrypt(requestHeader))).toHaveLength(32);
  });

  it('字符串类型的 body 不走 JSON.stringify（与请求层 switch 分支一致）', () => {
    const key = randomStr(32);
    const rawStringBody = 'already-a-string';
    const cipher = aes.encrypt(rawStringBody, key);
    expect(aes.decrypt(cipher, key)).toBe(rawStringBody);
  });

  it('每个请求使用独立的一次性 AES 密钥', () => {
    const keys = new Set(
      Array.from({ length: 100 }, () => frontend.encrypt(encodeBase64(randomStr(32)))),
    );
    expect(keys.size).toBe(100);
  });
});
