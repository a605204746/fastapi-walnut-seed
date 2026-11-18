import { $t } from '@vben/locales';
import { Col, Row, Tooltip } from 'antd';

import { accountBindList, handleAuthBinding } from '../oauth-common';

/**
 * 第三方登录行（对应 Vue 侧 views/_core/authentication/oauth-login.vue）。
 * 作为登录页 #third-party-login 插槽内容。
 */
export function OAuthLogin() {
  return (
    <div className="w-full sm:mx-auto md:max-w-md">
      <div className="my-4 flex items-center justify-between">
        <span className="border-input w-[35%] border-b dark:border-gray-600"></span>
        <span className="text-muted-foreground text-center text-xs uppercase">
          {$t('authentication.thirdPartyLogin')}
        </span>
        <span className="border-input w-[35%] border-b dark:border-gray-600"></span>
      </div>
      <Row className="enter-x flex items-center justify-evenly">
        {accountBindList.map((item) => (
          <Col className="my-2" key={item.source} span={4}>
            <Tooltip title={`${item.title}登录`}>
              <span className="flex cursor-pointer items-center justify-center">
                <img
                  className="size-[24px]"
                  onClick={() => handleAuthBinding(item.source)}
                  src={item.icon}
                />
              </span>
            </Tooltip>
          </Col>
        ))}
      </Row>
    </div>
  );
}

export default OAuthLogin;
