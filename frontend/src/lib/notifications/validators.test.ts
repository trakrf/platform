import { describe, it, expect } from "vitest";
import {
  normalizePhone,
  validatePhone,
  validateEmail,
  validateContact,
  splitPhone,
  joinPhone,
  validatePhoneParts,
  validateContactForm,
} from "./validators";

describe("notification contact validators", () => {
  describe("normalizePhone()", () => {
    it.each([
      ["+1 (555) 123-4567", "+15551234567"],
      ["  +44 20.7946.0958 ", "+442079460958"],
      ["+15551234567", "+15551234567"],
      ["", ""],
    ])("%j -> %j", (raw, expected) => {
      expect(normalizePhone(raw)).toBe(expected);
    });
  });

  describe("validatePhone()", () => {
    it.each([
      ["valid E.164", "+15551234567"],
      ["formatted input that normalizes", "+1 (555) 123-4567"],
      ["dotted input that normalizes", "+44 20.7946.0958"],
      ["minimum length (8 digits)", "+12345678"],
      ["maximum length (15 digits)", "+123456789012345"],
    ])("accepts %s", (_label, raw) => {
      expect(validatePhone(raw)).toBeNull();
    });

    it.each([
      ["missing +", "15551234567"],
      ["leading 0 country code", "+05551234567"],
      ["too short", "+1234567"],
      ["too long", "+1234567890123456"],
      ["letters", "+1555CALLNOW"],
      ["empty", ""],
    ])("rejects %s", (_label, raw) => {
      expect(validatePhone(raw)).toMatch(/\+<country code>/);
    });
  });

  describe("validateEmail()", () => {
    it.each(["a@b.co", "jane.doe+alerts@example.com", "  ops@trakrf.id  "])(
      "accepts %j",
      (raw) => {
        expect(validateEmail(raw)).toBeNull();
      },
    );

    it.each([
      "",
      "plain",
      "no-at.example.com",
      "a@b",
      "@example.com",
      "a b@example.com",
      "a@@b.co",
    ])("rejects %j", (raw) => {
      expect(validateEmail(raw)).not.toBeNull();
    });

    it("rejects an address over 320 characters", () => {
      expect(validateEmail(`${"a".repeat(310)}@example.com`)).toMatch(/320/);
    });
  });

  describe("validateContact()", () => {
    it.each([
      ["name + email", { name: "Jane", email: "jane@example.com", phone: "" }],
      ["name + phone", { name: "Jane", email: "", phone: "+1 (555) 123-4567" }],
      [
        "name + both",
        { name: "Jane", email: "jane@example.com", phone: "+15551234567" },
      ],
    ])("accepts %s", (_label, input) => {
      expect(validateContact(input)).toEqual({});
    });

    it.each([
      [
        "missing name",
        { name: "  ", email: "jane@example.com", phone: "" },
        ["name"],
      ],
      [
        "name too long",
        { name: "x".repeat(256), email: "jane@example.com", phone: "" },
        ["name"],
      ],
      [
        "no email or phone",
        { name: "Jane", email: " ", phone: "" },
        ["contact"],
      ],
      ["invalid email", { name: "Jane", email: "nope", phone: "" }, ["email"]],
      [
        "invalid phone",
        { name: "Jane", email: "", phone: "5551234567" },
        ["phone"],
      ],
      [
        "invalid email, valid phone",
        { name: "Jane", email: "nope", phone: "+15551234567" },
        ["email"],
      ],
      [
        "everything empty",
        { name: "", email: "", phone: "" },
        ["name", "contact"],
      ],
      [
        "everything invalid",
        { name: "", email: "nope", phone: "123" },
        ["name", "email", "phone"],
      ],
    ])("flags %s", (_label, input, fields) => {
      expect(Object.keys(validateContact(input)).sort()).toEqual(
        [...fields].sort(),
      );
    });

    it("uses the fixed contact message when both are empty", () => {
      expect(
        validateContact({ name: "Jane", email: "", phone: "" }).contact,
      ).toBe("Enter an email or a phone number");
    });
  });
});

describe("split phone helpers", () => {
  describe("splitPhone()", () => {
    it.each([
      ["+15551234567", "+1", "5551234567"],
      ["+447911123456", "+44", "7911123456"],
      ["+256772123456", "+256", "772123456"],
      ["+79161234567", "+7", "9161234567"],
      ["", "", ""],
      [undefined, "", ""],
    ])("%j -> %j + %j", (phone, code, number) => {
      expect(splitPhone(phone)).toEqual({ code, number });
    });
  });

  describe("joinPhone()", () => {
    it.each([
      ["+1", "555 123-4567", "+15551234567"],
      ["44", "(7911) 123.456", "+447911123456"],
      ["", "", ""],
    ])("%j + %j -> %j", (code, number, expected) => {
      expect(joinPhone({ code, number })).toBe(expected);
    });

    it("round-trips splitPhone()", () => {
      expect(joinPhone(splitPhone("+256772123456"))).toBe("+256772123456");
    });
  });

  describe("validatePhoneParts()", () => {
    it.each([
      ["both empty", "", ""],
      ["valid", "+1", "5551234567"],
      ["code without +", "44", "7911 123456"],
    ])("accepts %s", (_label, code, number) => {
      expect(validatePhoneParts({ code, number })).toEqual({});
    });

    it.each([
      ["missing code", "", "5551234567", { phoneCode: "Enter the country code, e.g. +1" }],
      ["code too long", "+1234", "5551234", { phoneCode: "Use 1 to 3 digits, e.g. +44" }],
      ["code with letters", "+1a", "5551234567", { phoneCode: "Use 1 to 3 digits, e.g. +44" }],
      ["missing number", "+1", "", { phoneNumber: "Enter the phone number" }],
      ["letters in number", "+1", "555CALLNOW", { phoneNumber: "Use digits only" }],
      ["leading 0", "+44", "07911123456", { phoneNumber: "Leave out the leading 0" }],
      ["too short", "+1", "555123", { phoneNumber: "This number is too short" }],
      ["too long", "+1", "5551234567890123", { phoneNumber: "This number is too long" }],
    ])("rejects %s", (_label, code, number, expected) => {
      expect(validatePhoneParts({ code, number })).toEqual(expected);
    });
  });

  describe("validateContactForm()", () => {
    it("accepts a phone-only contact", () => {
      expect(
        validateContactForm({ name: "Ann", email: "", phoneCode: "+1", phoneNumber: "5551234567" })
      ).toEqual({});
    });

    it("puts phone errors on the part that is wrong", () => {
      expect(
        validateContactForm({ name: "Ann", email: "", phoneCode: "+1", phoneNumber: "12" })
      ).toEqual({ phoneNumber: "This number is too short" });
    });

    it("still requires an email or a phone", () => {
      expect(
        validateContactForm({ name: "", email: "", phoneCode: "", phoneNumber: "" })
      ).toEqual({ name: "Name is required", contact: "Enter an email or a phone number" });
    });
  });
});
