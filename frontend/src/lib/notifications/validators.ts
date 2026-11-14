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

export type ContactField = "name" | "email" | "phone" | "contact";
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
