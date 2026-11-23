import JSEncrypt from 'jsencrypt';

import { BaseAsymmetricEncryption } from '../base';

/**
 * RSA 实现
 */
export class RsaEncryption extends BaseAsymmetricEncryption {
  override decrypt(str: string): string {
    const instance = new JSEncrypt();
    instance.setPrivateKey(this.privateKey);
    const ret = instance.decrypt(str);

    // jsencrypt 在密钥不匹配时返回 null（而非 false），
    // 两种失败都要拦下，否则 null 会流到 decodeBase64 抛出难定位的 TypeError。
    if (ret === false || ret === null) {
      throw new Error('RsaEncryption decrypt error');
    }

    return ret;
  }

  override encrypt(str: string): string {
    const instance = new JSEncrypt();
    instance.setPublicKey(this.publicKey);
    const ret = instance.encrypt(str);
    if (ret === false) {
      throw new Error('RsaEncryption encrypt error');
    }
    return ret;
  }
}
