export class ApiError extends Error {
  code: number;

  constructor(code: number, message: string) {
    super(message);
    this.code = code;
    this.name = 'ApiError';
  }

  isNotLogin(): boolean {
    return this.code === 40100;
  }

  isNoAuth(): boolean {
    return this.code === 40101;
  }

  isParamsError(): boolean {
    return this.code === 40000;
  }

  isForbidden(): boolean {
    return this.code === 40300;
  }
}
