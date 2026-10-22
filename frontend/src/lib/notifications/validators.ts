/**
 * Notification Contact Validation Functions
 *
 * Pure validation functions for notification contacts (recipients).
 * Return error string on failure, null on success.
 */

const E164_PATTERN = /^\+[1-9]\d{7,14}$/;
const EMAIL_PATTERN = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

export const NAME_MAX_LENGTH = 255;
export const EMAIL_MAX_LENGTH = 320;

/**
 * Strips common phone formatting (spaces, '-', '(', ')', '.') so
 * "+1 (555) 123-4567" becomes "+15551234567".
 *
 * @param raw - Phone number as typed
 * @returns Normalized phone number
 */
export function normalizePhone(raw: string): string {
  return raw.trim().replace(/[\s\-().]/g, "");
}

/**
 * Validates a phone number in E.164 format after normalization.
 *
 * @param raw - Phone number as typed
 * @returns Error message or null if valid
 */
export function validatePhone(raw: string): string | null {
  if (E164_PATTERN.test(normalizePhone(raw))) {
    return null;
  }
  return "Enter the phone number in international format: +<country code> then the number, e.g. +15551234567";
}

// ITU calling codes are 1, 2 or 3 digits and prefix-free: these are the only
// 1- and 2-digit ones, so any other code is 3 digits.
const ONE_DIGIT_CODES = new Set(["1", "7"]);
const TWO_DIGIT_CODES = new Set(
  "20 27 30 31 32 33 34 36 39 40 41 43 44 45 46 47 48 49 51 52 53 54 55 56 57 58 60 61 62 63 64 65 66 81 82 84 86 90 91 92 93 94 95 98".split(
    " "
  )
);

export interface PhoneParts {
  code: string;
  number: string;
}

const digitsOnly = (raw: string) => raw.replace(/\D/g, "");

/**
 * Splits a stored E.164 number into its country code and the rest, so
 * "+447911123456" becomes { code: "+44", number: "7911123456" }.
 *
 * @param phone - Stored phone number, possibly empty
 * @returns Country code (with "+") and national number
 */
export function splitPhone(phone: string | undefined): PhoneParts {
  const digits = digitsOnly(phone ?? "");
  if (!digits) return { code: "", number: "" };
  const length = ONE_DIGIT_CODES.has(digits.slice(0, 1))
    ? 1
    : TWO_DIGIT_CODES.has(digits.slice(0, 2))
      ? 2
      : 3;
  return { code: `+${digits.slice(0, length)}`, number: digits.slice(length) };
}

/**
 * Joins a country code and number into E.164, ignoring formatting.
 *
 * @returns E.164 number, or "" when both parts are empty
 */
export function joinPhone({ code, number }: PhoneParts): string {
  const digits = digitsOnly(code) + digitsOnly(number);
  return digits ? `+${digits}` : "";
}

export type PhoneErrors = { phoneCode?: string; phoneNumber?: string };

/**
 * Validates a phone number entered as a country code plus the rest. Both
 * empty is valid (no phone); otherwise each part gets its own message.
 *
 * @returns Field errors; empty object when valid
 */
export function validatePhoneParts({ code, number }: PhoneParts): PhoneErrors {
  const rawCode = code.trim();
  const rawNumber = number.trim();
  if (!rawCode && !rawNumber) return {};

  const errors: PhoneErrors = {};
  if (!rawCode) {
    errors.phoneCode = "Enter the country code, e.g. +1";
  } else if (!/^\+?[1-9]\d{0,2}$/.test(rawCode)) {
    errors.phoneCode = "Use 1 to 3 digits, e.g. +44";
  }

  const national = rawNumber.replace(/[\s\-().]/g, "");
  if (!national) {
    errors.phoneNumber = "Enter the phone number";
  } else if (!/^\d+$/.test(national)) {
    errors.phoneNumber = "Use digits only";
  } else if (national.startsWith("0")) {
    errors.phoneNumber = "Leave out the leading 0";
  } else if (!errors.phoneCode) {
    const total = digitsOnly(rawCode).length + national.length;
    if (total < 8) errors.phoneNumber = "This number is too short";
    else if (total > 15) errors.phoneNumber = "This number is too long";
  }
  return errors;
}

/**
 * Validates an email address (simple shape check, trimmed first).
 *
 * @param raw - Email as typed
 * @returns Error message or null if valid
 */
export function validateEmail(raw: string): string | null {
  const email = raw.trim();
  if (email.length > EMAIL_MAX_LENGTH) {
    return `Email must be ${EMAIL_MAX_LENGTH} characters or less`;
  }
  if (!EMAIL_PATTERN.test(email)) {
    return "Enter a valid email address";
  }
  return null;
}

export type ContactField =
  | "name"
  | "email"
  | "phone"
  | "phoneCode"
  | "phoneNumber"
  | "contact";
export type ContactErrors = Partial<Record<ContactField, string>>;

export interface ContactInput {
  name: string;
  email: string;
  phone: string;
}

/**
 * Validates a contact form. Name is required; email and phone are each
 * optional but must be valid when present, and at least one is required.
 *
 * @param input - Contact form values
 * @returns Field errors; empty object when valid
 */
export function validateContact({
  name,
  email,
  phone,
}: ContactInput): ContactErrors {
  const errors: ContactErrors = {};

  const trimmedName = name.trim();
  if (trimmedName.length === 0) {
    errors.name = "Name is required";
  } else if (trimmedName.length > NAME_MAX_LENGTH) {
    errors.name = `Name must be ${NAME_MAX_LENGTH} characters or less`;
  }

  const hasEmail = email.trim().length > 0;
  const hasPhone = phone.trim().length > 0;

  if (hasEmail) {
    const emailError = validateEmail(email);
    if (emailError) errors.email = emailError;
  }
  if (hasPhone) {
    const phoneError = validatePhone(phone);
    if (phoneError) errors.phone = phoneError;
  }
  if (!hasEmail && !hasPhone) {
    errors.contact = "Enter an email or a phone number";
  }

  return errors;
}

export interface ContactFormInput {
  name: string;
  email: string;
  phoneCode: string;
  phoneNumber: string;
}

/**
 * Validates a contact form whose phone is split into country code and
 * number. Phone errors land on the part that is wrong.
 *
 * @param input - Contact form values
 * @returns Field errors; empty object when valid
 */
export function validateContactForm({
  name,
  email,
  phoneCode,
  phoneNumber,
}: ContactFormInput): ContactErrors {
  const parts = { code: phoneCode, number: phoneNumber };
  const errors = validateContact({ name, email, phone: joinPhone(parts) });
  delete errors.phone;
  return { ...errors, ...validatePhoneParts(parts) };
}
