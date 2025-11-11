import { Router } from 'express';
import cors from 'cors';
import { cidrRestriction } from '../../middleware/cidr.js';
import { modCors } from '../../config/cors.js';
import { getPlainPassword } from '../../utils/auth.js';
import { checkRateLimit, generateRateLimitKey } from '../../utils/rateLimit.js';
import { actionHandlers } from './modHandlers.js';

const router = Router();

// Mod APIキーはアプリのログインパスワードと同じものを使用
const modApiKey = getPlainPassword();

const RATE_LIMIT_CONFIG = {
  windowMs: 15 * 60 * 1000,
  maxRequests: 1000,
  skipSuccessfulRequests: false
} as const;

const isJsonRequest = (req: any): boolean => {
  return typeof req.is === 'function' && req.is('application/json');
};

const respond = (res: any, status: number, payload: any) => {
  const body = {
    ...payload,
    timestamp: new Date().toISOString()
  };
  return res.status(status).json(body);
};

const validateRequest = (req: any) => {
  if (!isJsonRequest(req)) {
    return {
      status: 415,
      error: {
        code: 'UNSUPPORTED_MEDIA_TYPE',
        message: 'Content-Type must be application/json'
      }
    };
  }

  const { version, timestamp, apiKey, action } = req.body ?? {};

  if (version !== 1) {
    return {
      status: 400,
      error: {
        code: 'INVALID_VERSION',
        message: 'version must be 1'
      }
    };
  }

  if (typeof timestamp !== 'string' || timestamp.length === 0) {
    return {
      status: 400,
      error: {
        code: 'INVALID_TIMESTAMP',
        message: 'timestamp is required'
      }
    };
  }

  if (typeof apiKey !== 'string' || apiKey.length === 0) {
    return {
      status: 401,
      error: {
        code: 'INVALID_API_KEY',
        message: 'Invalid API key'
      }
    };
  }

  if (typeof action !== 'string' || action.length === 0) {
    return {
      status: 400,
      error: {
        code: 'INVALID_ACTION',
        message: 'action is required'
      }
    };
  }

  if (apiKey !== modApiKey) {
    return {
      status: 401,
      error: {
        code: 'INVALID_API_KEY',
        message: 'Invalid API key'
      }
    };
  }

  return null;
};

router.use(cors(modCors));
router.use(cidrRestriction);

router.post('/', async (req, res) => {
  const rateLimitKey = generateRateLimitKey(req);
  const rateLimitResult = checkRateLimit(rateLimitKey, RATE_LIMIT_CONFIG);

  res.set({
    'X-RateLimit-Limit': RATE_LIMIT_CONFIG.maxRequests.toString(),
    'X-RateLimit-Remaining': rateLimitResult.remaining.toString(),
    'X-RateLimit-Reset': new Date(rateLimitResult.resetTime).toISOString()
  });

  if (!rateLimitResult.allowed) {
    return respond(res, 429, {
      ok: false,
      action: req.body?.action ?? null,
      requestId: req.body?.requestId,
      error: {
        code: 'RATE_LIMIT_EXCEEDED',
        message: 'Rate limit exceeded. Please try again later.'
      }
    });
  }

  const validation = validateRequest(req);
  if (validation) {
    return respond(res, validation.status, {
      ok: false,
      action: req.body?.action ?? null,
      requestId: req.body?.requestId,
      error: validation.error
    });
  }

  const { action, params, requestId } = req.body;

  // アクションハンドラーの取得
  const handler = actionHandlers[action];
  if (!handler) {
    return respond(res, 404, {
      ok: false,
      action,
      requestId,
      error: {
        code: 'UNKNOWN_ACTION',
        message: `Unknown action: ${action}`
      }
    });
  }

  try {
    // ハンドラーを実行
    const data = await handler(params || {});
    return respond(res, 200, {
      ok: true,
      action,
      requestId,
      data
    });
  } catch (error) {
    console.error('[ModAPI] Action execution error:', {
      action,
      message: error instanceof Error ? error.message : String(error)
    });

    return respond(res, 500, {
      ok: false,
      action,
      requestId,
      error: {
        code: 'INTERNAL_ERROR',
        message: 'Failed to execute action'
      }
    });
  }
});

export { router as modRoutes };
