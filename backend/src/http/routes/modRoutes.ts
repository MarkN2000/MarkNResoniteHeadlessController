import { Router } from 'express';
import type { Request, Response } from 'express';
import cors from 'cors';
import { processManager } from '../../services/processManager.js';
import { cidrRestriction } from '../../middleware/cidr.js';
import { lenientRateLimit } from '../../middleware/rateLimit.js';
import { modCors } from '../../config/cors.js';
import { getPlainPassword } from '../../utils/auth.js';

const router = Router();

const modApiKey = process.env.MOD_API_KEY || getPlainPassword();

interface ModRequestBody {
  version: number;
  timestamp: string;
  apiKey: string;
  action: string;
  params?: unknown;
  requestId?: string;
}

type ErrorCode = 'UNKNOWN_ACTION' | 'GENERAL_ERROR';

const sendSuccess = (res: Response, action: string, requestId: string | undefined, data: unknown) => {
  const response: Record<string, unknown> = {
    ok: true,
    action,
    timestamp: new Date().toISOString(),
    data
  };

  if (requestId) {
    response.requestId = requestId;
  }

  res.status(200).json(response);
};

const sendError = (
  res: Response,
  status: number,
  action: string,
  requestId: string | undefined,
  code: ErrorCode,
  message: string
) => {
  const response: Record<string, unknown> = {
    ok: false,
    action,
    timestamp: new Date().toISOString(),
    error: { code, message }
  };

  if (requestId) {
    response.requestId = requestId;
  }

  res.status(status).json(response);
};

router.use(cors(modCors));
router.use(cidrRestriction);
router.use(lenientRateLimit);

router.post('/', async (req: Request, res: Response) => {
  if (!req.is('application/json')) {
    return sendError(res, 415, 'unknown', undefined, 'GENERAL_ERROR', 'Content-Type must be application/json');
  }

  const body = req.body as Partial<ModRequestBody> | undefined;
  const action = typeof body?.action === 'string' && body.action.trim() !== '' ? body.action : 'unknown';
  const requestId = typeof body?.requestId === 'string' ? body.requestId : undefined;

  if (body?.version !== 1) {
    return sendError(res, 400, action, requestId, 'GENERAL_ERROR', 'Invalid or missing version');
  }

  if (typeof body?.timestamp !== 'string' || Number.isNaN(Date.parse(body.timestamp))) {
    return sendError(res, 400, action, requestId, 'GENERAL_ERROR', 'Invalid or missing timestamp');
  }

  if (typeof body?.apiKey !== 'string' || body.apiKey.trim() === '') {
    return sendError(res, 401, action, requestId, 'GENERAL_ERROR', 'Invalid or missing API key');
  }

  if (!body?.action || typeof body.action !== 'string' || body.action.trim() === '') {
    return sendError(res, 400, 'unknown', requestId, 'GENERAL_ERROR', 'Invalid or missing action');
  }

  if (body.apiKey !== modApiKey) {
    return sendError(res, 401, action, requestId, 'GENERAL_ERROR', 'Invalid API key');
  }

  switch (body.action) {
    case 'sessionlist': {
      try {
        const result = await processManager.executeCommand('worlds');
        return sendSuccess(res, body.action, requestId, { result });
      } catch (error) {
        console.error('Mod sessionlist error:', error);
        return sendError(res, 500, body.action, requestId, 'GENERAL_ERROR', 'Failed to execute sessionlist');
      }
    }

    default:
      return sendError(res, 404, body.action, requestId, 'UNKNOWN_ACTION', 'Action is not supported');
  }
});

export { router as modRoutes };
