import { describe, it, expect } from "vitest";
import {
  normalizePhone,
  validatePhone,
  validateEmail,
  validateContact,
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
