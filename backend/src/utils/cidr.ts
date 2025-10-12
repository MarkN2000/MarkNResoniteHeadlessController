import fs from 'fs';
import path from 'path';

interface SecurityConfig {
  allowedCidrs: string[];
}

let securityConfig: SecurityConfig | null = null;

const loadSecurityConfig = (): SecurityConfig => {
  if (!securityConfig) {
    const configPath = path.join(process.cwd(), '..', 'config', 'security.json');
    try {
      const configData = fs.readFileSync(configPath, 'utf-8');
      securityConfig = JSON.parse(configData);
      console.log('Security config loaded:', securityConfig);
    } catch (error) {
      console.error('Failed to load security config:', error);
      console.error('Config path:', configPath);
      console.error('File exists:', fs.existsSync(configPath));
      
      // デフォルト設定を使用
      securityConfig = {
        allowedCidrs: ['192.168.0.0/16', '10.0.0.0/8']
      };
      console.log('Using default security config:', securityConfig);
    }
  }
  // 型ガード: 必ずSecurityConfigを返す
  if (!securityConfig) {
    securityConfig = { allowedCidrs: ['192.168.0.0/16', '10.0.0.0/8'] };
  }
  return securityConfig;
};

/**
 * IPアドレスがCIDR範囲内かどうかをチェック
 */
export const isIpInCidr = (ip: string, cidr: string): boolean => {
  const [network, prefixLength] = cidr.split('/');
  if (!network || !prefixLength) return false;
  
  const prefix = parseInt(prefixLength, 10);
  
  // IPv4アドレスを32ビット整数に変換
  const ipToInt = (ip: string): number => {
    return ip.split('.').reduce((acc, octet) => (acc << 8) + parseInt(octet, 10), 0) >>> 0;
  };
  
  const ipInt = ipToInt(ip);
  const networkInt = ipToInt(network);
  const mask = (0xFFFFFFFF << (32 - prefix)) >>> 0;
  
  return (ipInt & mask) === (networkInt & mask);
};

/**
 * IPアドレスが許可されたCIDR範囲内かどうかをチェック
 */
export const isIpAllowed = (ip: string): boolean => {
  const config = loadSecurityConfig();
  
  // ローカルホストは常に許可（開発環境対応）
  if (ip === '127.0.0.1' || ip === '::1' || ip === '::ffff:127.0.0.1' || ip === 'localhost') {
    console.log(`[CIDR] Allowing localhost: ${ip}`);
    return true;
  }
  
  // IPv6ローカルホストのバリエーション
  if (ip.startsWith('::ffff:127.0.0.1') || ip.includes('::1')) {
    console.log(`[CIDR] Allowing IPv6 localhost: ${ip}`);
    return true;
  }
  
  // 許可されたCIDR範囲をチェック
  const allowed = config.allowedCidrs.some(cidr => {
    try {
      return isIpInCidr(ip, cidr);
    } catch (error) {
      console.error(`[CIDR] Error checking ${ip} against ${cidr}:`, error);
      return false;
    }
  });
  
  if (allowed) {
    console.log(`[CIDR] IP ${ip} allowed by CIDR rules`);
  } else {
    console.log(`[CIDR] IP ${ip} not allowed by CIDR rules`);
  }
  
  return allowed;
};

/**
 * クライアントのIPアドレスを取得（プロキシ対応）
 */
export const getClientIp = (req: any): string => {
  // プロキシ経由の場合のIP取得
  const forwarded = req.headers['x-forwarded-for'];
  const realIp = req.headers['x-real-ip'];
  const remoteAddress = req.connection?.remoteAddress || req.socket?.remoteAddress;
  
  if (forwarded) {
    // X-Forwarded-For: client, proxy1, proxy2
    return forwarded.split(',')[0].trim();
  }
  
  if (realIp) {
    return realIp;
  }
  
  if (remoteAddress) {
    // IPv6マッピングされたIPv4アドレスを処理
    if (remoteAddress.startsWith('::ffff:')) {
      return remoteAddress.substring(7);
    }
    return remoteAddress;
  }
  
  return '127.0.0.1'; // フォールバック
};

/**
 * セキュリティログを記録
 */
export const logSecurityEvent = (event: string, ip: string, details?: any) => {
  const timestamp = new Date().toISOString();
  const logEntry = {
    timestamp,
    event,
    ip,
    details
  };
  
  console.log(`[SECURITY] ${timestamp} - ${event} from ${ip}`, details ? JSON.stringify(details) : '');
};

/**
 * 開発環境でのデバッグ情報を出力
 */
export const debugClientInfo = (req: any) => {
  const clientIp = getClientIp(req);
  const isAllowed = isIpAllowed(clientIp);
  
  console.log(`[DEBUG] Client IP: ${clientIp}`);
  console.log(`[DEBUG] Is Allowed: ${isAllowed}`);
  console.log(`[DEBUG] Headers:`, {
    'x-forwarded-for': req.headers['x-forwarded-for'],
    'x-real-ip': req.headers['x-real-ip'],
    'user-agent': req.headers['user-agent']
  });
  
  return { clientIp, isAllowed };
};
