import { request } from '@/lib/request';

export interface CaptchaResponse {
  captchaEnabled: boolean;
  img: string;
  uuid: string;
}

/** 图片验证码 */
export function captchaImage() {
  return request.get<CaptchaResponse>('/auth/code');
}

/** 发送短信验证码 */
export function sendSmsCode(phonenumber: string) {
  return request.get<void>('/resource/sms/code', { params: { phonenumber } });
}

/** 发送邮件验证码 */
export function sendEmailCode(email: string) {
  return request.get<void>('/resource/email/code', { params: { email } });
}
